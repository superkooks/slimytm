package main

import (
	"encoding/json"
	"os"
)

type PersistentData struct {
	// Map of MACs to names
	Clients map[string]PersistentClient `json:"clients"`
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
			panic(err)
		}
	} else if err != nil {
		panic(err)
	}
	defer f.Close()

	d := json.NewDecoder(f)
	err = d.Decode(&persistent)
	if err != nil {
		panic(err)
	}
}

func SavePersistent() {
	f, err := os.Create(PERSISTENT_LOCATION)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	e := json.NewEncoder(f)
	err = e.Encode(&persistent)
	if err != nil {
		panic(err)
	}
}
