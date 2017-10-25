package storage

import (
	"encoding/json"
	"fmt"
	"github.com/chenglch/consoleserver/common"
	"io/ioutil"
	"path"
	"reflect"
)

const (
	STORAGE_FILE = "file"
)

var (
	nodeConfigFile string
	nodeBackupFile string
)

func init() {
	STORAGE_INIT_MAP[STORAGE_FILE] = newFileStorage
}

type FileStorage struct {
	*Storage
	persistence uint32 // 0 no pending data, 1 has pending data
	pending     chan bool
}

func newFileStorage() StorInterface {
	stor := new(Storage)
	stor.async = false
	stor.Nodes = make(map[string]*Node)
	fileStor := new(FileStorage)
	fileStor.Storage = stor
	fileStor.persistence = 0
	fileStor.pending = make(chan bool, 1) // make it non-block
	return fileStor
}

func (s *FileStorage) ImportNodes() {
	nodeConfigFile = path.Join(serverConfig.Console.DataDir, "nodes.json")
	useBackup := false
	if ok, _ := common.PathExists(nodeConfigFile); ok {
		bytes, err := ioutil.ReadFile(nodeConfigFile)
		if err != nil {
			plog.Error(fmt.Sprintf("Could not read node configration file %s.", nodeConfigFile))
			useBackup = true
		}
		if err := json.Unmarshal(bytes, &s.Nodes); err != nil {
			plog.Error(fmt.Sprintf("Could not parse node configration file %s.", nodeConfigFile))
			useBackup = true
		}
	} else {
		useBackup = true
	}
	if !useBackup {
		return
	}
	nodeBackupFile = path.Join(serverConfig.Console.DataDir, "nodes.json.bak")
	if ok, _ := common.PathExists(nodeBackupFile); ok {
		plog.Info(fmt.Sprintf("Trying to load node bakup file %s.", nodeBackupFile))
		bytes, err := ioutil.ReadFile(nodeBackupFile)
		if err != nil {
			plog.Error(fmt.Sprintf("Could not read nonde backup file %s.", nodeBackupFile))
			return
		}
		if err := json.Unmarshal(bytes, &s.Nodes); err != nil {
			plog.Error(fmt.Sprintf("Could not parse node backup file %s.", nodeBackupFile))
			return
		}
		go func() {
			// as primary file can not be loaded, copy it from backup file
			// TODO: use rename instead of copy
			_, err = common.CopyFile(nodeConfigFile, nodeBackupFile)
			if err != nil {
				plog.Error(fmt.Sprintf("Unexpected error: %s, exit.", err))
				panic(err)
			}
		}()
	}
}

func (s *FileStorage) NotifyPersist(nodes interface{}, action int) {
	if reflect.TypeOf(nodes).Kind() == reflect.Map {
		s.Nodes = nodes.(map[string]*Node)
		common.Notify(s.pending, &s.persistence, 1)
	} else {
		plog.Error("Undefine persistance type")
	}
}

// a separate thread to save the data, avoid of frequent IO
func (s *FileStorage) PersistWatcher(eventChan chan map[int][]byte) {
	common.Wait(s.pending, &s.persistence, 0, s.save)
}

func (s *FileStorage) save() {
	var data []byte
	var err error
	if data, err = json.Marshal(s.Nodes); err != nil {
		plog.Error(fmt.Sprintf("Could not Marshal the node map: %s.", err))
		panic(err)
	}
	nodeConfigFile = path.Join(serverConfig.Console.DataDir, "nodes.json")
	nodeBackupFile = path.Join(serverConfig.Console.DataDir, "nodes.json.bak")
	if ok, _ := common.PathExists(nodeConfigFile); ok {
		// TODO: Use rename instead of copy
		_, err = common.CopyFile(nodeBackupFile, nodeConfigFile)
		if err != nil {
			plog.Error(fmt.Sprintf("Unexpected error: %s, exit.", err))
			panic(err)
		}
	}
	err = common.WriteJsonFile(nodeConfigFile, data)
	if err != nil {
		plog.Error(fmt.Sprintf("Unexpected error: %s, exit.", err))
		panic(err)
	}
	go func() {
		_, err = common.CopyFile(nodeBackupFile, nodeConfigFile)
		if err != nil {
			plog.Error(fmt.Sprintf("Unexpected error: %s, exit.", err))
		}
	}()
}

func (s *FileStorage) IsAsync() bool {
	return false
}

func (s *FileStorage) ListNodeWithHost() map[string]string {
	return nil
}