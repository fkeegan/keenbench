package rpc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"keenbench/engine/internal/logging"
)

const (
	jsonRPCVersion = "2.0"
	rpcErrorCode   = -32000
	maxMessageSize = 10 * 1024 * 1024
)

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	APIVer  string          `json:"api_version,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *ErrorPayload   `json:"error,omitempty"`
}

type Notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type ErrorPayload struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type Handler func(ctx context.Context, params json.RawMessage) (any, *Error)

type Error struct {
	Message string
	Data    interface{}
}

type Server struct {
	apiVersion string
	reader     *bufio.Reader
	writer     *bufio.Writer
	mu         sync.Mutex
	handlers   map[string]Handler
	logger     *slog.Logger
}

func NewServer(apiVersion string, r io.Reader, w io.Writer, logger *slog.Logger) *Server {
	if logger == nil {
		logger = logging.Nop()
	}
	return &Server{
		apiVersion: apiVersion,
		reader:     bufio.NewReader(r),
		writer:     bufio.NewWriter(w),
		handlers:   make(map[string]Handler),
		logger:     logger,
	}
}

func (s *Server) Register(method string, handler Handler) {
	s.handlers[method] = handler
}

func (s *Server) Serve(ctx context.Context) error {
	for {
		line, err := s.reader.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			s.logger.Error("rpc.read_failed", "error", err.Error())
			return err
		}
		if len(line) == 0 {
			continue
		}
		if len(line) > maxMessageSize {
			s.logger.Warn("rpc.message_too_large", "bytes", len(line))
			s.sendError(nil, "message too large", nil)
			continue
		}
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			s.logger.Warn("rpc.invalid_json", "error", err.Error())
			s.sendError(nil, "invalid json", nil)
			continue
		}
		if req.JSONRPC != jsonRPCVersion {
			s.logger.Warn("rpc.invalid_version", "version", req.JSONRPC)
			s.sendError(req.ID, "invalid jsonrpc version", nil)
			continue
		}
		if req.APIVer != "" && req.APIVer != s.apiVersion {
			s.logger.Warn("rpc.incompatible_version", "requested", req.APIVer, "expected", s.apiVersion)
			s.sendError(req.ID, "incompatible api_version", map[string]string{"expected": s.apiVersion})
			continue
		}
		handler, ok := s.handlers[req.Method]
		if !ok {
			s.logger.Warn("rpc.method_not_found", "method", req.Method)
			s.sendError(req.ID, fmt.Sprintf("method not found: %s", req.Method), nil)
			continue
		}
		s.logger.Debug("rpc.request", "method", req.Method, "id", string(req.ID), "params", logging.RedactJSON(req.Params))
		go s.handleRequest(ctx, req, handler)
	}
}

func (s *Server) handleRequest(ctx context.Context, req Request, handler Handler) {
	result, err := handler(ctx, req.Params)
	if req.ID == nil {
		return
	}
	if err != nil {
		s.logger.Error("rpc.response_error", "method", req.Method, "id", string(req.ID), "error", logging.RedactAny(err.Data))
		s.sendError(req.ID, err.Message, err.Data)
		return
	}
	s.logger.Debug("rpc.response", "method", req.Method, "id", string(req.ID), "result", logging.RedactAny(result))
	resp := Response{JSONRPC: jsonRPCVersion, ID: req.ID, Result: result}
	s.send(resp)
}

func (s *Server) Notify(method string, params any) {
	s.logger.Debug("rpc.notify", "method", method, "params", logging.RedactAny(params))
	n := Notification{JSONRPC: jsonRPCVersion, Method: method, Params: params}
	s.send(n)
}

func (s *Server) sendError(id json.RawMessage, message string, data interface{}) {
	resp := Response{
		JSONRPC: jsonRPCVersion,
		ID:      id,
		Error:   &ErrorPayload{Code: rpcErrorCode, Message: message, Data: data},
	}
	s.send(resp)
}

func (s *Server) send(payload any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = s.writer.Write(append(data, '\n'))
	_ = s.writer.Flush()
}
