package common

import (
	"net/http"

	log "github.com/sirupsen/logrus"
)

type Logger struct {
	plog *log.Entry
}

func GetLogger(pkg string) *Logger {
	log.SetLevel(log.DebugLevel)
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

func (l *Logger) ErrorNode(node string, message interface{}) {
	l.plog.WithFields(log.Fields{"node": node}).Error(message)
}

func (l *Logger) WarningNode(node string, message interface{}) {
	l.plog.WithFields(log.Fields{"node": node}).Warning(message)
}

func (l *Logger) DebugNode(node string, message interface{}) {
	l.plog.WithFields(log.Fields{"node": node}).Debug(message)
}

func (l *Logger) InfoNode(node string, message interface{}) {
	l.plog.WithFields(log.Fields{"node": node}).Info(message)
}

func (l *Logger) Info(message string) {
	l.plog.Info(message)
}

func (l *Logger) Error(err interface{}) {
	l.plog.Error(err)
}

func (l *Logger) Debug(err interface{}) {
	l.plog.Debug(err)
}
