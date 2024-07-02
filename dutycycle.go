package main

import "time"

type DutyCycle struct {
	Period time.Duration
	DutyCycle time.Duration
	dutyCyclePercent float64
	State bool
}

func NewDutyCycle(period time.Duration, dutyCyclePercent float64) *DutyCycle {
	dutyCycle := time.Duration((float64(dutyCyclePercent) / 100.0) * float64(period))
	return &DutyCycle{
		Period: period,
		DutyCycle: dutyCycle,
		dutyCyclePercent: dutyCyclePercent,
	}
}

func (d *DutyCycle) SetPeriod(period time.Duration) {
	d.Period = period
	d.DutyCycle = time.Duration((d.dutyCyclePercent / 100.0) * float64(period))
}

func (d *DutyCycle) SetDutyCycle(dutyCycle time.Duration) {
	d.DutyCycle = dutyCycle
	pct := float64(dutyCycle / d.Period * 100)
	d.dutyCyclePercent = pct
}

func (d *DutyCycle) GetPeriod() time.Duration {
	return d.Period
}

func (d *DutyCycle) GetDutyCycle() time.Duration {
	return d.DutyCycle
}

func (d *DutyCycle) SetDutyCyclePercent(percent float64) {
	d.dutyCyclePercent = percent
	d.DutyCycle = time.Duration((percent / 100.0) * float64(d.Period))
}

func (d *DutyCycle) Start(onFunc func(), offFunc func()) {
	var heatingFunc func()
	heatingFunc = func () {
		if d.DutyCycle == 0 {
			offFunc()
		} else {
			d.State = true
			onFunc()
		}
		time.AfterFunc(d.DutyCycle, func() {
			if d.DutyCycle != d.Period {
				d.State = false
				offFunc()
				time.AfterFunc(d.Period - d.DutyCycle, heatingFunc)
			} else {
				time.AfterFunc(d.Period, heatingFunc)
			}
		})
	}
	heatingFunc()
}

func (d *DutyCycle) GetState() bool {
	return d.State
}