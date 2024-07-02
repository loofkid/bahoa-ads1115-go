package main

import (
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"
)

type Config struct {
	Pid struct {
		ProportionalGain float64
		IntegralGain float64
		DerivativeGain float64
	}
	DutyCycle struct {
		Period float64
	}
	ProcessTime int
	LogLocation string
}

func LoadConfig() *Config {
	viper.SetDefault("pid.proportionalGain", 3.0)
	viper.SetDefault("pid.integralGain", 0.0)
	viper.SetDefault("pid.derivativeGain", 0.0)

	viper.SetDefault("dutyCycle.period", 5.0)

	viper.SetDefault("processTime", 200)

	viper.SetDefault("logLocation", "/var/log/bahoa/")


	viper.SetConfigName("config")
	viper.AddConfigPath("/opt/bahoa/")
	viper.SetConfigType("yaml")

	err := viper.ReadInConfig()
	if err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found; ignore error since defaults are set
		} else {
			panic(err)
		}
	}

	viper.WriteConfig()

	config := &Config{}

	config.Pid.ProportionalGain = viper.GetFloat64("pid.proportionalGain")
	config.Pid.IntegralGain = viper.GetFloat64("pid.integralGain")
	config.Pid.DerivativeGain = viper.GetFloat64("pid.derivativeGain")

	config.DutyCycle.Period = viper.GetFloat64("dutyCycle.period")

	config.ProcessTime = viper.GetInt("processTime")

	config.LogLocation = viper.GetString("logLocation")

	viper.OnConfigChange(func(e fsnotify.Event) {
		config.Pid.ProportionalGain = viper.GetFloat64("pid.proportionalGain")
		config.Pid.IntegralGain = viper.GetFloat64("pid.integralGain")
		config.Pid.DerivativeGain = viper.GetFloat64("pid.derivativeGain")

		config.DutyCycle.Period = viper.GetFloat64("dutyCycle.period")

		config.ProcessTime = viper.GetInt("processTime")

		config.LogLocation = viper.GetString("logLocation")
	})

	viper.WatchConfig()

	return config
}

func (c *Config) WriteConfig() {
	viper.Set("pid.proportionalGain", c.Pid.ProportionalGain)
	viper.Set("pid.integralGain", c.Pid.IntegralGain)
	viper.Set("pid.derivativeGain", c.Pid.DerivativeGain)

	viper.Set("dutyCycle.period", c.DutyCycle.Period)

	viper.Set("processTime", c.ProcessTime)

	viper.Set("logLocation", c.LogLocation)

	viper.WriteConfig()
}