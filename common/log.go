package common

import (
	"fmt"
	"net/http"
	"os"

	log "github.com/sirupsen/logrus"
)

func init() {
	log.SetFormatter(&log.JSONFormatter{})
	file, err := os.OpenFile(fmt.Sprintf("logs/%s.log", os.Args[0]), os.O_CREATE|os.O_WRONLY, 0666)
	if err == nil {
		log.SetOutput(file)
	} else {
		log.Info("Failed to log to file, using default stderr")
	}
	log.SetLevel(log.InfoLevel)
}

type Logger struct {
	plog *log.Entry
}

func GetLogger(pkg string) *Logger {
	return &Logger{plog: log.WithFields(log.Fields{"pkg": pkg})}
}

func (l *Logger) HandleHttp(w http.ResponseWriter, req *http.Request, code int, err error) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	l.plog.WithFields(log.Fields{"code": code, "method": req.Method, "uri": req.Method})
	if err != nil {
		l.plog.Error(err.Error())
		w.WriteHeader(code)
	} else {
		l.plog.Info("OK")
		w.WriteHeader(http.StatusOK)
	}
}

func (l *Logger) ErrorNode(node string, message string) {
	l.plog.WithFields(log.Fields{"node": node}).Error(message)
}

func (l *Logger) InfoNode(node string, message string) {
	l.plog.WithFields(log.Fields{"node": node}).Info(message)
}

func (l *Logger) Info(message string) {
	l.plog.Info(message)
}

func (l *Logger) Error(err error) {
	l.plog.Error(err.Error())
}
