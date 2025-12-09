package stratum

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"juno-pool/internal/config"
	"juno-pool/internal/db"
	"juno-pool/internal/job"
)

// Server provides a minimal TLS stratum listener; protocol handling to be implemented.
type Server struct {
	cfg        config.Config
	store      *db.Store
	listener   net.Listener
	mu         sync.Mutex
	shutting   bool
	waitGroup  sync.WaitGroup
	extraCtr   uint32
	source     job.Source
	submitter  *job.Submitter
	current    atomic.Value // *job.Template
	jobs       map[string]*job.Template
	jobsMu     sync.Mutex
	sessions   map[*Session]struct{}
	sessionsMu sync.Mutex
}

func NewServer(cfg config.Config, store *db.Store) *Server {
	src, err := job.NewRPCSource(cfg.NodeRPCURL)
	if err != nil {
		log.Printf("warn: rpc source init failed: %v", err)
	}
	sub, err := job.NewSubmitter(cfg.NodeRPCURL)
	if err != nil {
		log.Printf("warn: rpc submitter init failed: %v", err)
	}
	s := &Server{cfg: cfg, store: store, source: src, submitter: sub}
	s.current.Store((*job.Template)(nil))
	s.jobs = make(map[string]*job.Template)
	s.sessions = make(map[*Session]struct{})
	return s
}

// Start begins listening for stratum connections.
func (s *Server) Start() error {
	var ln net.Listener
	var err error

	// Use TLS if cert/key paths are provided
	if s.cfg.TLSCertPath != "" && s.cfg.TLSKeyPath != "" {
		cert, err := tls.LoadX509KeyPair(s.cfg.TLSCertPath, s.cfg.TLSKeyPath)
		if err != nil {
			return fmt.Errorf("load tls keys: %w", err)
		}
		ln, err = tls.Listen("tcp", s.cfg.StratumListen, &tls.Config{Certificates: []tls.Certificate{cert}})
		if err != nil {
			return fmt.Errorf("listen: %w", err)
		}
		log.Printf("stratum listening on %s (TLS)", s.cfg.StratumListen)
	} else {
		// Plain TCP without TLS
		ln, err = net.Listen("tcp", s.cfg.StratumListen)
		if err != nil {
			return fmt.Errorf("listen: %w", err)
		}
		log.Printf("stratum listening on %s (no TLS)", s.cfg.StratumListen)
	}

	s.mu.Lock()
	s.listener = ln
	s.shutting = false
	s.mu.Unlock()

	s.waitGroup.Add(1)
	go s.acceptLoop()

	// Start template polling in background if source available.
	if s.source != nil {
		s.waitGroup.Add(1)
		go s.templateLoop()
	}
	return nil
}

// Stop shuts down the listener and waits for connection handlers to exit.
func (s *Server) Stop() error {
	s.mu.Lock()
	s.shutting = true
	if s.listener != nil {
		_ = s.listener.Close()
	}
	s.mu.Unlock()

	s.waitGroup.Wait()
	return nil
}

func (s *Server) acceptLoop() {
	defer s.waitGroup.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if s.isShutting() {
				return
			}
			log.Printf("accept error: %v", err)
			continue
		}
		s.waitGroup.Add(1)
		go func(c net.Conn) {
			defer s.waitGroup.Done()
			s.handleConn(c)
		}(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()
	extranonce1 := s.nextExtranonce()
	sess := NewSession(s.cfg, conn, extranonce1, s.getTemplate, s.getJob, s.submitter, s.store)
	s.registerSession(sess)
	sess.Serve()
	s.unregisterSession(sess)
}

func (s *Server) nextExtranonce() string {
	val := atomic.AddUint32(&s.extraCtr, 1)
	return fmt.Sprintf("%08x", val)
}

func (s *Server) registerSession(sess *Session) {
	s.sessionsMu.Lock()
	s.sessions[sess] = struct{}{}
	s.sessionsMu.Unlock()
}

func (s *Server) unregisterSession(sess *Session) {
	s.sessionsMu.Lock()
	delete(s.sessions, sess)
	s.sessionsMu.Unlock()
}

func (s *Server) templateLoop() {
	defer s.waitGroup.Done()
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	ctx := context.Background()
	for {
		if s.isShutting() {
			return
		}
		tmpl, err := s.source.Next(ctx)
		if err != nil {
			log.Printf("template fetch error: %v", err)
		} else if tmpl != nil {
			s.current.Store(tmpl)
			s.storeJob(tmpl)
			s.broadcastTemplate(tmpl)
		}
		select {
		case <-t.C:
		case <-ctx.Done():
			return
		}
	}
}

func (s *Server) getTemplate() *job.Template {
	val := s.current.Load()
	if val == nil {
		return nil
	}
	return val.(*job.Template)
}

func (s *Server) storeJob(tmpl *job.Template) {
	s.jobsMu.Lock()
	defer s.jobsMu.Unlock()
	s.jobs[tmpl.JobID] = tmpl
	// Trim map if it grows too large (keep last ~32 jobs).
	if len(s.jobs) > 32 {
		for k := range s.jobs {
			delete(s.jobs, k)
			if len(s.jobs) <= 16 {
				break
			}
		}
	}
}

func (s *Server) getJob(id string) *job.Template {
	s.jobsMu.Lock()
	defer s.jobsMu.Unlock()
	return s.jobs[id]
}

func (s *Server) broadcastTemplate(tmpl *job.Template) {
	s.sessionsMu.Lock()
	sessions := make([]*Session, 0, len(s.sessions))
	for sess := range s.sessions {
		sessions = append(sessions, sess)
	}
	s.sessionsMu.Unlock()

	for _, sess := range sessions {
		_ = sess.PushTemplate(tmpl)
	}
}

func (s *Server) isShutting() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shutting
}

// ConnectedCount returns the number of connected miners.
func (s *Server) ConnectedCount() int {
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	return len(s.sessions)
}

// GetSessionStats returns connected count and total hashrate.
func (s *Server) GetSessionStats() (count int, hashrate float64) {
	// Copy sessions under lock, then release before calling Hashrate()
	s.sessionsMu.Lock()
	sessions := make([]*Session, 0, len(s.sessions))
	for sess := range s.sessions {
		sessions = append(sessions, sess)
	}
	s.sessionsMu.Unlock()

	count = len(sessions)
	for _, sess := range sessions {
		hashrate += sess.Hashrate()
	}
	return
}
