package common

import (
	"fmt"
	"os"
)

type SignalHandler func(s os.Signal, arg interface{})

type signalSet struct {
	m map[os.Signal]SignalHandler
}

func SignalSetNew() *signalSet {
	ss := new(signalSet)
	ss.m = make(map[os.Signal]SignalHandler)
	return ss
}

func (set *signalSet) Register(s os.Signal, handler SignalHandler) {
	if _, found := set.m[s]; !found {
		set.m[s] = handler
	}
}

func (set *signalSet) Handle(sig os.Signal, arg interface{}) (err error) {
	if _, found := set.m[sig]; found {
		set.m[sig](sig, arg)
		return nil
	} else {
		return fmt.Errorf("No handler available for signal %v", sig)
	}
}

func (set *signalSet) GetSigMap() map[os.Signal]SignalHandler {
	return set.m
}
