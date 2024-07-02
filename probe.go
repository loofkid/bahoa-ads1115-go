package main

import (
	"math"
)

type Probe struct {
	Id				string
	Name 			string
	setTempK		float64
	address			int
	channel			int
	adc 			*ADC
	resistorValue 	float64
	smoothingWindow int
	lastReadings	[]float64
}

func NewProbe(id string, 
			  name string, 
			  address int, 
			  channel int, 
			  resistorValue float64,
			  adc *ADC,
			  smoothingWindow int) *Probe {
	return &Probe{
		Id: id,
		Name: name,
		setTempK: 0.0,
		address: address,
		channel: channel,
		resistorValue: resistorValue,
		adc: adc,
		lastReadings: []float64{},
		smoothingWindow: smoothingWindow,
	}
}

func (p *Probe) ReadTempK() (float64, error) {
	analogReading, err := p.adc.ADC.ReadWithDefaults(p.channel)
	if err != nil {
		return 0.0, err
	}
	aValue := 0.0007343140544
	bValue := 0.0002157437229
	cValue := 0.0000000951568577


	resistance := p.resistorValue * (p.adc.NominalVoltage / analogReading - 1.0)
	temperature := 1.0 / (aValue + bValue * math.Log(resistance) + cValue * math.Pow(math.Log(resistance), 3.0))

	// Smoothing
	// fmt.Println("Count of readings: ", len(p.lastReadings))
	if (len(p.lastReadings) <= p.smoothingWindow) {
		// fmt.Println("Less than smoothing window (", p.smoothingWindow, "), filling slice")
		p.lastReadings = append(p.lastReadings, temperature)
	} else {
		// fmt.Println("At smoothing window (", p.smoothingWindow, "), shifting slice")
		p.lastReadings = append(p.lastReadings[1:], temperature)
	}
	// fmt.Println("New count of readings: ", len(p.lastReadings))
	total := 0.0
	for _, reading := range p.lastReadings {
		total += reading
	}
	temperature = total / float64(len(p.lastReadings))

	return temperature, nil
}

func (p *Probe) ReadTempC() (float64, error) {
	temperature, err := p.ReadTempK()
	if err != nil {
		return 0.0, err
	}
	return temperature - 273.15, nil
}

func (p *Probe) ReadTempF() (float64, error) {
	temperature, err := p.ReadTempC()
	if err != nil {
		return 0.0, err
	}
	return temperature * 9.0 / 5.0 + 32.0, nil
}

func (p *Probe) SetTempK(tempK float64) {
	p.setTempK = tempK
}

func (p *Probe) SetTempC(tempC float64) {
	p.setTempK = tempC + 273.15
}

func (p *Probe) SetTempF(tempF float64) {
	p.setTempK = (tempF - 32.0) * 5.0 / 9.0
}

func (p *Probe) GetSetTempK() float64 {
	return p.setTempK
}

func (p *Probe) GetSetTempC() float64 {
	if p.setTempK == 0.0 {
		return 0.0
	}
	return p.setTempK - 273.15
}

func (p *Probe) GetSetTempF() float64 {
	if p.setTempK == 0.0 {
		return 0.0
	}
	return p.GetSetTempC() * 9.0 / 5.0 + 32.0
}

func (p *Probe) ReadConnected() bool {
	temperature, err := p.ReadTempK()
	if err != nil {
		return false
	}
	return temperature >= 270.0 && temperature <= 500.0
}