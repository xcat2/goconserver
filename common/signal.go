package common

import (
	"fmt"
	"os"
	"os/signal"
)

var (
	signalSet *SignalSet
)

func DoSignal() {
	s := GetSignalSet()
	for {
		c := make(chan os.Signal)
		var sigs []os.Signal
		for sig := range s.GetSigMap() {
			sigs = append(sigs, sig)
		}
		signal.Notify(c)
		sig := <-c
		err := s.Handle(sig, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unknown signal received: %v\n", sig)
			os.Exit(1)
		}
	}
}

type SignalHandler func(s os.Signal, arg interface{})

type SignalSet struct {
	m map[os.Signal]SignalHandler
}

func GetSignalSet() *SignalSet {
	if signalSet == nil {
		signalSet = new(SignalSet)
		signalSet.m = make(map[os.Signal]SignalHandler)
	}
	return signalSet
}

func (set *SignalSet) Register(s os.Signal, handler SignalHandler) {
	if _, found := set.m[s]; !found {
		set.m[s] = handler
	}
}

func (set *SignalSet) Handle(sig os.Signal, arg interface{}) (err error) {
	if _, found := set.m[sig]; found {
		set.m[sig](sig, arg)
		return nil
	} else {
		return fmt.Errorf("No handler available for signal %v", sig)
	}
}

func (set *SignalSet) GetSigMap() map[os.Signal]SignalHandler {
	return set.m
}
