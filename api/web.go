package api

import (
	"compress/gzip"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/xcat2/goconserver/console"
	"golang.org/x/net/websocket"
	"io"
	"net/http"
	"strings"
)

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

func MakeGzipHandler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if the client can accept the gzip encoding.
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			handler.ServeHTTP(w, r)
			return
		}

		// Set the HTTP header indicating encoding.
		w.Header().Set("Content-Encoding", "gzip")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		gzw := gzipResponseWriter{Writer: gz, ResponseWriter: w}
		handler.ServeHTTP(gzw, r)
	})
}

func WebHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		plog.Info(fmt.Sprintf("Receive %s request %s from %s.", r.Method, r.URL.Path, r.RemoteAddr))
		if r.URL.EscapedPath() == "/" || r.URL.EscapedPath() == "/index.html" {
			w.Header().Add("Cache-Control", "no-store")
		}
		http.FileServer(http.Dir(serverConfig.API.DistDir)).ServeHTTP(w, r)
	})
}

func RegisterBackendHandler(router *mux.Router) {
	router.PathPrefix("/session").Handler(websocket.Handler(websocketHandler))
	router.PathPrefix("/").Handler(MakeGzipHandler(WebHandler()))
}

func websocketHandler(ws *websocket.Conn) {
	plog.Info(fmt.Sprintf("Recieve websocket request from %s\n", ws.RemoteAddr().String()))
	console.AcceptWesocketClient(ws)
}
