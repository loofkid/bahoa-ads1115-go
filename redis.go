package main

import (
	"log"
	"time"

	redistimeseries "github.com/RedisTimeSeries/redistimeseries-go"
)

type Redis struct {
	client 				*redistimeseries.Client
}

func NewRedis(host, port string, password *string) *Redis {
	client := redistimeseries.NewClient(host + ":" + port, "bahoa-redis", password)
	return &Redis{client: client}
}

func (r *Redis) CreateTS(key string, avg bool) {
	_, exists := r.client.Info(key)

	if exists != nil {
		r.client.CreateKeyWithOptions(key, redistimeseries.DefaultCreateOptions)
		if avg {
			r.client.CreateKeyWithOptions(key + "_avg", redistimeseries.DefaultCreateOptions)
			r.client.CreateRule(key, redistimeseries.AvgAggregation, 50, key + "_avg")
		}
	}
}

func (r *Redis) AddEntry(key string, value float64) {
	r.client.AddAutoTs(key, value)
}

func (r *Redis) GetAvg(key string) float64 {
	value, err := r.client.Get(key)
	if err != nil {
		log.Println(err)
		return 0
	}
	return value.Value
}

func (r *Redis) GetRecentEntries(probeId string, since time.Duration) []map[string]interface{} {
	now := time.Now()
	startTime := now.Add(-since).UnixMilli()
	endTime := now.UnixMilli()
	tempValues, tempErr := r.client.RangeWithOptions(probeId + "-temp", startTime, endTime, redistimeseries.DefaultRangeOptions)
	setValues, setErr := r.client.RangeWithOptions(probeId + "-set-temp", startTime, endTime, redistimeseries.DefaultRangeOptions)
	if tempErr != nil {
		log.Println(tempErr)
		return nil
	}
	if setErr != nil {
		log.Println(setErr)
		return nil
	}

	data := []map[string]interface{}{}
	for i, value := range tempValues {
		var setValue float64
		if len(setValues) == 0 {
			setValue = 0
		} else if len(setValues) <= i {
			setValue = setValues[len(setValues) - 1].Value
		} else {
			setValue = setValues[i].Value
		}
		data = append(data, map[string]interface{} {
			"probeId": probeId,
			"temperature": value.Value,
			"timestamp": value.Timestamp,
			"setTemperature": setValue,
		})
	}
	return data
}

