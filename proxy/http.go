package proxy

import (
	"bufio"
	"io"
	"log/slog"
	"net/http"

	"github.com/chenen3/yeager/flow"
	"github.com/chenen3/yeager/transport"
)

type httpHandler struct {
	dialer transport.Dialer
}

func NewHTTPHandler(dialer transport.Dialer) *httpHandler {
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
	targetConn, err := h.dialer.Dial(proxyReq.Context(), proxyReq.Host)
	if err != nil {
		http.Error(proxyResp, "Failed to connect target", http.StatusServiceUnavailable)
		slog.Error(err.Error())
		return
	}
	defer targetConn.Close()

	hijacker, ok := proxyResp.(http.Hijacker)
	if !ok {
		http.Error(proxyResp, "Failed to hijack", http.StatusInternalServerError)
		return
	}
	proxyConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(proxyResp, "Failed to hijack connection", http.StatusInternalServerError)
		slog.Error(err.Error())
		return
	}
	defer proxyConn.Close()

	// inform the client
	_, err = proxyConn.Write([]byte("HTTP/1.1 200 Connection established\r\n\r\n"))
	if err != nil {
		slog.Error(err.Error())
		return
	}

	go func() {
		flow.Copy(targetConn, proxyConn)
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
		slog.Error(err.Error())
		return
	}
	defer targetConn.Close()

	err = proxyReq.Write(targetConn)
	if err != nil {
		http.Error(proxyResp, "Failed to send request", http.StatusServiceUnavailable)
		slog.Error(err.Error())
		return
	}
	targetResp, err := http.ReadResponse(bufio.NewReader(targetConn), proxyReq)
	if err != nil {
		http.Error(proxyResp, "Failed to read target response", http.StatusServiceUnavailable)
		slog.Error(err.Error())
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
		slog.Error(err.Error())
		return
	}
}
