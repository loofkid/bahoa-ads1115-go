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
	DataLocation string
	Redis struct {
		Host string
		Port string
		Password string
	}
	LocalAuth string
}

func LoadConfig() *Config {
	viper.SetEnvPrefix("bahoa")
	viper.AutomaticEnv()

	viper.SetDefault("pid.proportionalGain", 3.0)
	viper.SetDefault("pid.integralGain", 0.0)
	viper.SetDefault("pid.derivativeGain", 0.0)

	viper.SetDefault("dutyCycle.period", 5.0)

	viper.SetDefault("processTime", 200)

	viper.SetDefault("logLocation", "/var/log/bahoa/")

	viper.SetDefault("dataLocation", "/opt/bahoa/data/")

	viper.SetDefault("redis.host", "10.13.13.11")
	viper.SetDefault("redis.port", "6379")
	viper.SetDefault("redis.password", "")


	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("/opt/bahoa/")
	viper.AddConfigPath("$HOME/.bahoa")
	viper.AddConfigPath(".")

	err := viper.ReadInConfig()
	if err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found; ignore error since defaults are set
		} else {
			panic(err)
		}
	}

	errWrite := viper.WriteConfigAs("/opt/bahoa/config.yaml")
	if errWrite != nil {
		panic(errWrite)
	}

	config := &Config{}

	config.Pid.ProportionalGain = viper.GetFloat64("pid.proportionalGain")
	config.Pid.IntegralGain = viper.GetFloat64("pid.integralGain")
	config.Pid.DerivativeGain = viper.GetFloat64("pid.derivativeGain")

	config.DutyCycle.Period = viper.GetFloat64("dutyCycle.period")

	config.ProcessTime = viper.GetInt("processTime")

	config.LogLocation = viper.GetString("logLocation")

	config.DataLocation = viper.GetString("dataLocation")

	config.Redis.Host = viper.GetString("redis.host")
	config.Redis.Port = viper.GetString("redis.port")
	config.Redis.Password = viper.GetString("redis.password")

	config.LocalAuth = viper.GetString("localauthpw")

	viper.OnConfigChange(func(e fsnotify.Event) {
		config.Pid.ProportionalGain = viper.GetFloat64("pid.proportionalGain")
		config.Pid.IntegralGain = viper.GetFloat64("pid.integralGain")
		config.Pid.DerivativeGain = viper.GetFloat64("pid.derivativeGain")

		config.DutyCycle.Period = viper.GetFloat64("dutyCycle.period")

		config.ProcessTime = viper.GetInt("processTime")

		config.LogLocation = viper.GetString("logLocation")

		config.DataLocation = viper.GetString("dataLocation")

		config.Redis.Host = viper.GetString("redis.host")
		config.Redis.Port = viper.GetString("redis.port")
		config.Redis.Password = viper.GetString("redis.password")

		config.LocalAuth = viper.GetString("localauthpw")
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

	viper.Set("dataLocation", c.DataLocation)

	viper.Set("redis.host", c.Redis.Host)
	viper.Set("redis.port", c.Redis.Port)
	viper.Set("redis.password", c.Redis.Password)

	viper.WriteConfig()
}