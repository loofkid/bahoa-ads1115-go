package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"

	"go.einride.tech/pid"

	"gobot.io/x/gobot/v2"
	// "gobot.io/x/gobot/v2/drivers/gpio"
	"gobot.io/x/gobot/v2/drivers/i2c"

	"github.com/stianeikeland/go-rpio/v4"

	// "gobot.io/x/gobot/v2/platforms/adaptors"
	"gobot.io/x/gobot/v2/platforms/raspi"

	sio "github.com/zishang520/socket.io/v2/socket"
)

type TempSetData struct {
	ProbeId string
	SetTemp float64
}

func main() {
	config := LoadConfig()
	defer config.WriteConfig()

	os.MkdirAll(config.LogLocation, 0755)

	logger := &lumberjack.Logger{
		Filename: config.LogLocation + "bahoa.log",
		MaxSize: 200, // megabytes
		MaxBackups: 3,
		MaxAge: 28, // days
		Compress: true,
	}

	redis := NewRedis(config.Redis.Host, config.Redis.Port, &config.Redis.Password)

	redis.CreateTS("probe-0-temp", true)
	redis.CreateTS("probe-1-temp", true)
	redis.CreateTS("probe-2-temp", true)
	redis.CreateTS("probe-3-temp", true)
	redis.CreateTS("probe-0-set-temp", false)
	redis.CreateTS("probe-1-set-temp", false)
	redis.CreateTS("probe-2-set-temp", false)
	redis.CreateTS("probe-3-set-temp", false)

	mw := io.MultiWriter(os.Stdout, logger)

	log.SetOutput(mw)
	defer logger.Close()

	wifi := NewWifi()
	defer wifi.Close()

	raspiAdaptor := raspi.NewAdaptor()
	raspiAdaptor.Connect()

	ads1115driver := i2c.NewADS1115Driver(raspiAdaptor, i2c.WithADS1x15BestGainForVoltage(3.3))

	dutyCycle := NewDutyCycle(time.Duration(config.DutyCycle.Period)*time.Second, 0)

	rpio.Open()

	heatPin := rpio.Pin(22)
	heatPin.Output()
	heatPin.Low()
	defer heatPin.Low()
	dutyCycle.Start(heatPin.High, heatPin.Low)

	pidController := pid.Controller{
		Config: pid.ControllerConfig{
			ProportionalGain: config.Pid.ProportionalGain,
			IntegralGain: config.Pid.IntegralGain,
			DerivativeGain: config.Pid.DerivativeGain,
		},
	}

	probes := []Probe{}

	work := func() {

		port := "0.0.0.0:3000"

		socketServer := sio.NewServer(nil, nil)

		router := mux.NewRouter()

		router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			log.Println("Hello World!")
			w.Write([]byte("poopsicles!"))
		})

		router.Handle("/socket.io/", socketServer.ServeHandler(nil))

		socketServer.On("connection", func(clients ...any) {
			socket := clients[0].(*sio.Socket)

			socket.Emit("connected", "Connected to server")
			log.Printf("Socket %v connected\n", clients)
			socket.On("joinRoom", func(data ...any) {
				room := data[0].(string)
				socket.Join(sio.Room(room))
				// for _, room := range rooms {
				// 	log.Default().Println("Adding client", socket.Id(), "to room", room)
				// 	socket.Join([]sio.Room(room.(string)))
				// }
			})

			socket.On("setTemp", func(data ...any) {
				log.Default().Println(data[0])
				tempSetData := data[0].(map[string]interface{})
				defer func() {
					if r := recover(); r != nil {
						log.Default().Println("Recovered in f", r)
					}
				}()
				log.Default().Println(tempSetData["ProbeId"])
				log.Default().Println(tempSetData["SetTemp"])
				probeIndex := slices.IndexFunc(probes, func(p Probe) bool { return p.Id == tempSetData["ProbeId"].(string) })
				
				if probeIndex != -1 {
					probes[probeIndex].SetTempK(tempSetData["SetTemp"].(float64))
				}
			})

			socket.On("getPID", func(a ...any) {
				pidData := map[string]float64{
					"p": pidController.Config.ProportionalGain,
					"i": pidController.Config.IntegralGain,
					"d": pidController.Config.DerivativeGain,
				}

				socket.Emit("PID", pidData)
			})

			socket.On("setPID", func(data ...any) {
				log.Default().Println(data[0])
				pidData := data[0].(map[string]interface{})
				defer func() {
					if r := recover(); r != nil {
						log.Default().Println("Recovered in f", r)
					}
				}()
				pidController.Config.ProportionalGain = pidData["p"].(float64)
				config.Pid.ProportionalGain = pidData["p"].(float64)
				pidController.Config.IntegralGain = pidData["i"].(float64)
				config.Pid.IntegralGain = pidData["i"].(float64)
				pidController.Config.DerivativeGain = pidData["d"].(float64)
				config.Pid.DerivativeGain = pidData["d"].(float64)
				config.WriteConfig()
				newPidData := map[string]float64{
					"p": pidController.Config.ProportionalGain,
					"i": pidController.Config.IntegralGain,
					"d": pidController.Config.DerivativeGain,
				}
				socket.Emit("PID", newPidData)
			})

			socket.On("getDutyCyclePeriod", func(a ...any) {
				socket.Emit("dutyCyclePeriod", float64(dutyCycle.GetPeriod()) / float64(time.Second))
			})

			socket.On("setDutyCyclePeriod", func(data ...any) {
				log.Default().Println(data[0])
				period := data[0].(float64)
				defer func() {
					if r := recover(); r != nil {
						log.Default().Println("Recovered in f", r)
					}
				}()
				log.Default().Println(period)
				dutyCycle.SetPeriod(time.Duration(period * 1000.0) * time.Millisecond)
				config.DutyCycle.Period = period
				config.WriteConfig()
				socket.Emit("dutyCyclePeriod", float64(dutyCycle.GetPeriod()) / float64(time.Second))
			})

			socket.On("getWifi", func(a ...any) {
				currentNetwork, err := wifi.GetCurrentNetwork()
				if err != nil {
					currentNetwork = Wlan{}
				}
				log.Default().Println(currentNetwork)
				socket.Emit("wifi", currentNetwork)
			})
		})

		router.Use(authMiddleware)

		gobot.Every(30*time.Second, func() {
			currentNetwork, err := wifi.GetCurrentNetwork()
			if err != nil {
				currentNetwork = Wlan{}
			}
			log.Default().Println(currentNetwork)
			socketServer.To("pi").Emit("wifi", currentNetwork)
		})


		adc := NewADC(3.3, ads1115driver)
		probeChannels := []int{0, 1, 2, 3}
		procTime := time.Duration(config.ProcessTime)*time.Millisecond
		gobot.Every(procTime, func() {
			for i, channel := range probeChannels {
				index := slices.IndexFunc(probes, func(p Probe) bool {
					return p.Id == fmt.Sprintf("probe-%v", i)
				})
				if index == -1 {
						if channel == 0 {
							probe := NewProbe(fmt.Sprintf("probe-%v", i), "Smoker", 0x48, channel, 100000, adc, 50)
							
							if (probe.ReadConnected()) {
								probes = append(probes, *probe)
							}
						} else {
							probe := NewProbe(fmt.Sprintf("probe-%v", i), fmt.Sprintf("Probe %v", i), 0x48, channel, 100000, adc, 50)
							if (probe.ReadConnected()) {
								probes = append(probes, *probe)
							}
						}
				} else {
					if !probes[index].ReadConnected() {
						probes = slices.DeleteFunc(probes, func(p Probe) bool { return p.Id == fmt.Sprintf("probe-%v", i) })
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
					redis.AddEntry(probe.Id + "-temp", tempK)
					redis.AddEntry(probe.Id + "-set-temp", probe.GetSetTempK())
					probeJson, _ := json.Marshal(map[string]interface{}{
						"id": probe.Id,
						"name": probe.Name,
						"tempK": roundFloat(tempK, 2),
						"tempC": roundFloat(tempC, 2),
						"tempF": roundFloat(tempF, 2),
						"setTempK": probe.GetSetTempK(),
						"setTempC": probe.GetSetTempC(),
						"setTempF": probe.GetSetTempF(),
						"avgTempK": redis.GetAvg(probe.Id + "-temp"),
					})
					socketServer.In("pi").Emit("probe", probeJson)
					socketServer.In("pi").Emit("probeRecords", redis.GetRecentEntries(probe.Id, 10 * time.Minute))
				}
			}
			if len(probes) > 0 {
				smokerProbeIndex := slices.IndexFunc(probes, func(p Probe) bool { return p.Id == "probe-0" })
				if smokerProbeIndex != -1 && probes[smokerProbeIndex].ReadConnected() {
					smokerProbe := probes[smokerProbeIndex]
					currentTemp, _ := smokerProbe.ReadTempK()
					setTemp := smokerProbe.GetSetTempK()
					if setTemp == 0 {
						// log.Default().Println("No set temp. Turning off heat.")
						dutyCycle.SetDutyCyclePercent(0)
					} else if (setTemp - currentTemp) > 10 {
						// log.Default().Println("Smoker low enough that PID doesn't matter. Turning on heat.")
						dutyCycle.SetDutyCyclePercent(100)
					} else {
						// log.Default().Println("Smoker close enough to PID. Switching to PID control.")
						pidController.Update(pid.ControllerInput{
							ReferenceSignal: setTemp,
							ActualSignal: currentTemp,
							SamplingInterval: procTime,
						})
						pidOutput := pidController.State.ControlSignal
						// log.Default().Printf("PID Error: %v\n", pidController.State.ControlError)
						// log.Default().Printf("PID Output: %v\n", pidOutput)
						scaledOutput := 0.0
						if pidOutput > 0 {
							scaledOutput = 0 * (1 - pidOutput / setTemp) + 100 * (pidOutput / setTemp)
						}
						// log.Default().Printf("Scaled Output: %v\n", scaledOutput)
						dutyCycle.SetDutyCyclePercent(scaledOutput)
						// log.Default().Printf("Duty Cycle: %v\n", dutyCycle.GetDutyCycle() / time.Millisecond)
					}
				}
			}
		})

		log.Printf("Listening on http://%v\n", port)
		log.Fatal(http.ListenAndServe(port, handlers.CORS(handlers.AllowedOrigins([]string{"*"}))(router)))
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


func authMiddleware(next http.Handler) http.Handler {
	allowedIPMap := map[string]bool{
		"127.0.0.1": true,
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		address := strings.Split(r.RemoteAddr, ":")[0]
		log.Printf("Request from %v\n", r.RemoteAddr)
		match, _ := regexp.MatchString(`[::1]`, r.RemoteAddr)
		if match || allowedIPMap[address] {
			next.ServeHTTP(w, r)
			return
		}

		http.Error(w, "Forbidden", http.StatusUnauthorized)
	})
}