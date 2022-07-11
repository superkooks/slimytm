package main

import (
	"encoding/json"
	"os"
)

type PersistentData struct {
	// Map of MACs to names
	Clients      map[string]PersistentClient `json:"clients"`
	LogLocations []string                    `json:"logLocations"`
}

type PersistentClient struct {
	Name string `json:"name"`
}

var persistent PersistentData

const PERSISTENT_LOCATION = "slimytm_persistent.json"

func LoadPersistent() {
	f, err := os.Open(PERSISTENT_LOCATION)
	if os.IsNotExist(err) {
		f, err = os.Create(PERSISTENT_LOCATION)
		if err != nil {
			logger.Panicw("unable to create persistent data file",
				"location", PERSISTENT_LOCATION,
				"err", err)
		}

		f.Write([]byte("{}"))
		f.Seek(0, 0)
	} else if err != nil {
		logger.Panicw("unable to open persistent data file",
			"location", PERSISTENT_LOCATION,
			"err", err)
	}
	defer f.Close()

	d := json.NewDecoder(f)
	err = d.Decode(&persistent)
	if err != nil {
		logger.Panicw("unable to parse persistent data",
			"location", PERSISTENT_LOCATION,
			"err", err)
	}
}

func SavePersistent() {
	f, err := os.Create(PERSISTENT_LOCATION)
	if err != nil {
		logger.DPanicw("unable to save persistent data",
			"location", PERSISTENT_LOCATION,
			"err", err)
		return
	}
	defer f.Close()

	e := json.NewEncoder(f)
	err = e.Encode(&persistent)
	if err != nil {
		logger.DPanicw("unable to encode persistent data",
			"location", PERSISTENT_LOCATION,
			"err", err)
	}
}
