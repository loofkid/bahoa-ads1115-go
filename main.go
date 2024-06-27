package main

import (
	"fmt"
	"time"
	"math"
	"slices"

	"gobot.io/x/gobot"
	"gobot.io/x/gobot/drivers/i2c"
	"gobot.io/x/gobot/platforms/raspi"
)

func main() {
	raspiAdaptor := raspi.NewAdaptor()

	ads1115driver := i2c.NewADS1115Driver(raspiAdaptor)

	work := func() {
		adc := NewADC(3.3, ads1115driver)
		probes := []Probe{}
		probeChannels := []int{0, 1, 2, 3}
		gobot.Every(200*time.Millisecond, func() {
			for i, channel := range probeChannels {
				if (slices.IndexFunc(probes, func(p Probe) bool {
					return p.Id == fmt.Sprintf("probe-%v", i + 1)
				}) == -1) {
					probe := NewProbe(fmt.Sprintf("probe-%v", i + 1), fmt.Sprintf("Probe %v", i), 0x48, channel, 100000, adc, 5)
					if (probe.ReadConnected()) {
						probes = append(probes, *probe)
					}
				}
			}
			for _, probe := range probes {
				tempK, errK := probe.ReadTempK()
				tempC, _ := probe.ReadTempC()
				tempF, _ := probe.ReadTempF()
				if errK != nil {
					fmt.Println(errK)
				} else {
					fmt.Printf("%v: %vK\n", probe.Name, roundFloat(tempK, 2))
					fmt.Printf("%v: %vºC\n", probe.Name, roundFloat(tempC, 2))
					fmt.Printf("%v: %vºF\n", probe.Name, roundFloat(tempF, 2))
				}
			}
		})
	}

	robot := gobot.NewRobot("ads1115Bot",
		[]gobot.Connection{raspiAdaptor},
		[]gobot.Device{ads1115driver},
		work,
	)

	robot.Start()

}

func roundFloat(val float64, precision uint) float64 {
    ratio := math.Pow(10, float64(precision))
    return math.Round(val*ratio) / ratio
}