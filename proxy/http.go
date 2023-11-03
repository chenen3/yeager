package proxy

// This HTTP proxy is adapted from the httpproxy package in outline-sdk,
// which is more intuitive and clear than mine (written with TCP server).
// See details in https://github.com/Jigsaw-Code/outline-sdk/tree/main/x/httpproxy
// TODO: import the original package when it is officially released.

import (
	"bufio"
	"io"
	"net/http"
	"time"

	"github.com/chenen3/yeager/flow"
	"github.com/chenen3/yeager/logger"
	"github.com/chenen3/yeager/transport"
)

type httpHandler struct {
	dialer transport.StreamDialer
}

func NewHTTPHandler(dialer transport.StreamDialer) *httpHandler {
	return &httpHandler{dialer: dialer}
}

func (h httpHandler) ServeHTTP(proxyResp http.ResponseWriter, proxyReq *http.Request) {
	if proxyReq.Method == http.MethodConnect {
		h.connect(proxyResp, proxyReq)
		return
	}
	h.forward(proxyResp, proxyReq)
}

func (h httpHandler) connect(proxyResp http.ResponseWriter, proxyReq *http.Request) {
	if proxyReq.Host == "" {
		http.Error(proxyResp, "missing host", http.StatusBadRequest)
		return
	}
	if proxyReq.URL.Port() == "" {
		http.Error(proxyResp, "missing port in address", http.StatusBadRequest)
		return
	}
	start := time.Now()
	targetConn, err := h.dialer.Dial(proxyReq.Context(), proxyReq.Host)
	if err != nil {
		http.Error(proxyResp, "Failed to connect target", http.StatusServiceUnavailable)
		logger.Error.Print(err)
		return
	}
	defer targetConn.Close()
	logger.Debug.Printf("connect to %s, timed: %dms", proxyReq.Host, time.Since(start).Milliseconds())

	hijacker, ok := proxyResp.(http.Hijacker)
	if !ok {
		http.Error(proxyResp, "Failed to hijack", http.StatusInternalServerError)
		return
	}
	proxyConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(proxyResp, "Failed to hijack connection", http.StatusInternalServerError)
		logger.Error.Print(err)
		return
	}
	defer proxyConn.Close()

	// inform the client
	_, err = proxyConn.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
	if err != nil {
		logger.Error.Print(err)
		return
	}

	go func() {
		flow.Copy(targetConn, proxyConn)
		// unblock subsequent read
		targetConn.CloseWrite()
	}()
	flow.Copy(proxyConn, targetConn)
}

func (h httpHandler) forward(proxyResp http.ResponseWriter, proxyReq *http.Request) {
	if proxyReq.Host == "" {
		http.Error(proxyResp, "missing host", http.StatusBadRequest)
		return
	}
	host := proxyReq.Host
	if proxyReq.URL.Port() == "" {
		host += ":80"
	}
	targetConn, err := h.dialer.Dial(proxyReq.Context(), host)
	if err != nil {
		http.Error(proxyResp, "Failed to connect target", http.StatusServiceUnavailable)
		logger.Error.Print(err)
		return
	}
	defer targetConn.Close()

	err = proxyReq.Write(targetConn)
	if err != nil {
		http.Error(proxyResp, "Failed to send request", http.StatusServiceUnavailable)
		logger.Error.Print(err)
		return
	}
	targetResp, err := http.ReadResponse(bufio.NewReader(targetConn), proxyReq)
	if err != nil {
		http.Error(proxyResp, "Failed to read target response", http.StatusServiceUnavailable)
		logger.Error.Print(err)
		return
	}
	defer targetResp.Body.Close()

	for key, values := range targetResp.Header {
		for _, value := range values {
			proxyResp.Header().Add(key, value)
		}
	}
	_, err = io.Copy(proxyResp, targetResp.Body)
	if err != nil {
		http.Error(proxyResp, "Failed to write response", http.StatusServiceUnavailable)
		logger.Error.Print(err)
		return
	}
}
