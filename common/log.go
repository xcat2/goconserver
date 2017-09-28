package common

import (
	"net/http"

	"fmt"
	log "github.com/sirupsen/logrus"
	"runtime"
	"strings"
)

type Logger struct {
	plog *log.Entry
	pkg  string
}

func GetLogger(pkg string) *Logger {
	log.SetLevel(log.DebugLevel)
	return &Logger{plog: log.WithFields(log.Fields{}), pkg: pkg}
}

func (l *Logger) fileinfo() log.Fields {
	_, file, line, ok := runtime.Caller(2)
	if !ok {
		file = "???"
		line = 1
	} else {
		slash := strings.LastIndex(file, "/")
		if slash >= 0 {
			file = file[slash+1:]
		}
	}
	return log.Fields{"file": fmt.Sprintf("%s/%s (%d)", l.pkg, file, line)}
}

func (l *Logger) HandleHttp(w http.ResponseWriter, req *http.Request, code int, err error) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	l.plog.WithFields(log.Fields{"code": code, "method": req.Method, "uri": req.Method})
	if err != nil {
		l.plog.WithFields(l.fileinfo()).Error(err.Error())
		w.WriteHeader(code)
	} else {
		l.plog.WithFields(l.fileinfo()).Info("OK")
		w.WriteHeader(http.StatusOK)
	}
}

func (l *Logger) ErrorNode(node string, message interface{}) {
	l.plog.WithFields(log.Fields{"node": node}).WithFields(l.fileinfo()).Error(message)
}

func (l *Logger) WarningNode(node string, message interface{}) {
	l.plog.WithFields(log.Fields{"node": node}).WithFields(l.fileinfo()).Warning(message)
}

func (l *Logger) DebugNode(node string, message interface{}) {
	l.plog.WithFields(log.Fields{"node": node}).WithFields(l.fileinfo()).Debug(message)
}

func (l *Logger) InfoNode(node string, message interface{}) {
	l.plog.WithFields(log.Fields{"node": node}).WithFields(l.fileinfo()).Info(message)
}

func (l *Logger) Info(message string) {
	l.plog.WithFields(l.fileinfo()).Info(message)
}

func (l *Logger) Error(err interface{}) {
	l.plog.WithFields(l.fileinfo()).Error(err)
}

func (l *Logger) Debug(err interface{}) {
	l.plog.WithFields(l.fileinfo()).Debug(err)
}
