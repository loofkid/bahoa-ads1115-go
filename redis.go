package main

import (
	"log"
	"time"

	redistimeseries "github.com/RedisTimeSeries/redistimeseries-go"
)

type Redis struct {
	localClient 				*redistimeseries.Client
	remoteClient				*redistimeseries.Client
	localRetention 				time.Duration
}

type ProbeEntry struct {
	ProbeId         string  `json:"probeId"`
	Temperature     float64 `json:"temperature"`
	Timestamp       int64   `json:"timestamp"`
	SetTemperature  float64 `json:"setTemperature"`
}

func NewRedis(host, port string, password *string, localRetention time.Duration) *Redis {
	localClient := redistimeseries.NewClient("bahoa-redis:" + port, "bahoa-redis", password)
	remoteClient := redistimeseries.NewClient(host + ":" + port, "bahoa-redis-remote", password)
	
	return &Redis{
		localClient: localClient,
		remoteClient: remoteClient,
		localRetention: localRetention,
	}
}

func (r *Redis) CreateTS(key string, avg bool) {
	_, exists := r.localClient.Info(key)

	localOptions := redistimeseries.DefaultCreateOptions
	localOptions.RetentionMSecs = r.localRetention

	if exists != nil {
		r.localClient.CreateKeyWithOptions(key, localOptions)
		r.remoteClient.CreateKeyWithOptions(key, redistimeseries.DefaultCreateOptions)
		if avg {
			r.localClient.CreateKeyWithOptions(key + "_avg", localOptions)
			r.remoteClient.CreateKeyWithOptions(key + "_avg", redistimeseries.DefaultCreateOptions)
			r.localClient.CreateRule(key, redistimeseries.AvgAggregation, 50, key + "_avg")
			r.remoteClient.CreateRule(key, redistimeseries.AvgAggregation, 50, key + "_avg")
		}
	}
}

func (r *Redis) AddEntry(key string, value float64) {
	r.localClient.AddAutoTs(key, value)
	r.remoteClient.AddAutoTs(key, value)
}

func (r *Redis) GetAvg(key string) float64 {
	value, err := r.localClient.Get(key)
	if err != nil {
		log.Println(err)
		return 0
	}
	return value.Value
}

func (r *Redis) GetRecentEntries(probeId string, since time.Duration) []ProbeEntry {
	now := time.Now()
	startTime := now.Add(-since).UnixMilli()
	endTime := now.UnixMilli()
	var tempValues, setValues []redistimeseries.DataPoint
	var tempErr, setErr error
	if since > r.localRetention {
		log.Default().Println("Since is greater than local retention, reaching out to remote db")
		tempValues, tempErr = r.remoteClient.RangeWithOptions(probeId + "-temp", startTime, endTime, redistimeseries.DefaultRangeOptions)
		setValues, setErr = r.remoteClient.RangeWithOptions(probeId + "-set-temp", startTime, endTime, redistimeseries.DefaultRangeOptions)
	} else {
		// log.Default().Println("Since is less than local retention, using local db")
		tempValues, tempErr = r.localClient.RangeWithOptions(probeId + "-temp", startTime, endTime, redistimeseries.DefaultRangeOptions)
		setValues, setErr = r.localClient.RangeWithOptions(probeId + "-set-temp", startTime, endTime, redistimeseries.DefaultRangeOptions)
	}

	if tempErr != nil {
		log.Println(tempErr)
		return nil
	}
	if setErr != nil {
		log.Println(setErr)
		return nil
	}

	data := []ProbeEntry{}
	for i, value := range tempValues {
		var setValue float64
		if len(setValues) == 0 {
			setValue = 0
		} else if len(setValues) <= i {
			setValue = setValues[len(setValues) - 1].Value
		} else {
			setValue = setValues[i].Value
		}
		data = append(data, ProbeEntry{
			ProbeId: probeId,
			Temperature: value.Value,
			Timestamp: value.Timestamp,
			SetTemperature: setValue,
		})
	}
	return data
}

func (r *Redis) GetAllEntries(probeId string, local bool) []ProbeEntry {
	var tempValues, setValues []redistimeseries.DataPoint
	var tempErr, setErr error
	if local {
		log.Default().Println("Getting all entries from local db")
		tempValues, tempErr = r.localClient.RangeWithOptions(probeId + "-temp", 0, time.Now().UnixMilli(), redistimeseries.DefaultRangeOptions)
		setValues, setErr = r.localClient.RangeWithOptions(probeId + "-set-temp", 0, time.Now().UnixMilli(), redistimeseries.DefaultRangeOptions)
	} else {
		log.Default().Println("Getting all entries from remote db")
		tempValues, tempErr = r.remoteClient.RangeWithOptions(probeId + "-temp", 0, time.Now().UnixMilli(), redistimeseries.DefaultRangeOptions)
		setValues, setErr = r.remoteClient.RangeWithOptions(probeId + "-set-temp", 0, time.Now().UnixMilli(), redistimeseries.DefaultRangeOptions)
	}

	if tempErr != nil {
		log.Println(tempErr)
		return nil
	}
	if setErr != nil {
		log.Println(setErr)
		return nil
	}

	data := []ProbeEntry{}
	for i, value := range tempValues {
		var setValue float64
		if len(setValues) == 0 {
			setValue = 0
		} else if len(setValues) <= i {
			setValue = setValues[len(setValues) - 1].Value
		} else {
			setValue = setValues[i].Value
		}
		data = append(data, ProbeEntry{
			ProbeId: probeId,
			Temperature: value.Value,
			Timestamp: value.Timestamp,
			SetTemperature: setValue,
		})
	}
	return data
}

func (r *Redis) GetDataRange(probeId string, start time.Time, end time.Time) []ProbeEntry {
	var tempValues, setValues []redistimeseries.DataPoint
	var tempErr, setErr error
	if start.Before(time.Now().Add(-r.localRetention)) {
		log.Default().Println("Requested data from before local retention, reaching out to remote db")
		tempValues, tempErr = r.remoteClient.RangeWithOptions(probeId + "-temp", start.UnixMilli(), end.UnixMilli(), redistimeseries.DefaultRangeOptions)
		setValues, setErr = r.remoteClient.RangeWithOptions(probeId + "-set-temp", start.UnixMilli(), end.UnixMilli(), redistimeseries.DefaultRangeOptions)
	} else {
		log.Default().Println("Requested data from within local retention, using local db")
		tempValues, tempErr = r.localClient.RangeWithOptions(probeId + "-temp", start.UnixMilli(), end.UnixMilli(), redistimeseries.DefaultRangeOptions)
		setValues, setErr = r.localClient.RangeWithOptions(probeId + "-set-temp", start.UnixMilli(), end.UnixMilli(), redistimeseries.DefaultRangeOptions)
	}
	if tempErr != nil {
		log.Println(tempErr)
		return nil
	}
	if setErr != nil {
		log.Println(setErr)
		return nil
	}

	data := []ProbeEntry{}
	for i, value := range tempValues {
		var setValue float64
		if len(setValues) == 0 {
			setValue = 0
		} else if len(setValues) <= i {
			setValue = setValues[len(setValues) - 1].Value
		} else {
			setValue = setValues[i].Value
		}
		data = append(data, ProbeEntry{
			ProbeId: probeId,
			Temperature: value.Value,
			Timestamp: value.Timestamp,
			SetTemperature: setValue,
		})
	}
	return data
}