package main

import (
	"gobot.io/x/gobot/drivers/i2c"
)

type ADC struct {
	NominalVoltage 	float64
	ADC 			*i2c.ADS1x15Driver
}

func NewADC(nominalVoltage float64, adc *i2c.ADS1x15Driver) *ADC {
	return &ADC{
		NominalVoltage: nominalVoltage,
		ADC: adc,
	}
}