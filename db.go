package main

import (
	"encoding/json"

	"github.com/tidwall/buntdb"
)

type DB struct {
	DiskDB *buntdb.DB
	MemDB  *buntdb.DB
}

func NewDB(dataLocation string) *DB {
	diskDB, _ := buntdb.Open(dataLocation + "bahoa.db")
	memDB, _ := buntdb.Open(":memory:")

	defer diskDB.Close()
	defer memDB.Close()

	memDB.CreateIndex("timestamp", "*", buntdb.IndexJSON("timestamp"))

	return &DB{
		DiskDB: diskDB,
		MemDB:  memDB,
	}
}

func (db *DB) WriteReading(probeId string, reading float64, timestamp int64) {
	jsonRecord, _ := json.Marshal(map[string]interface{}{
		"reading":   reading,
		"timestamp": timestamp,
	})
	jsonString := string(jsonRecord)
	db.MemDB.Update(func(tx *buntdb.Tx) error {
		tx.Set(probeId, jsonString, nil)
		return nil
	})
}

func (db *DB) GetRecentReadings(probeId string, earliestTimestamp int64) []map[string]interface{} {
	criteria, _ := json.Marshal(map[string]interface{}{
		"timestamp": earliestTimestamp,
	})
	criteriaString := string(criteria)
	records := make([]map[string]interface{}, 0)
	db.MemDB.View(func(tx *buntdb.Tx) error {
		tx.AscendGreaterOrEqual("timestamp", criteriaString, func(key string, value string) bool {
			var record map[string]interface{}
			json.Unmarshal([]byte(value), &record)
			records = append(records, record)
			return true
		})
		return nil
	})
	return records
}