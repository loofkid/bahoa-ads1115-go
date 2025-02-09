package main

import (
	"fmt"
	"io"
	"log"
	"math"
	"os/signal"

	// "sync"
	"syscall"

	// "net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"

	// "github.com/gorilla/handlers"
	// "github.com/gorilla/mux"

	// "go.einride.tech/pid"

	"gobot.io/x/gobot/v2"
	// "gobot.io/x/gobot/v2/drivers/gpio"
	"gobot.io/x/gobot/v2/drivers/i2c"

	"github.com/stianeikeland/go-rpio/v4"

	// "gobot.io/x/gobot/v2/platforms/adaptors"
	"gobot.io/x/gobot/v2/platforms/raspi"

	"github.com/zishang520/engine.io/v2/types"
	sio "github.com/zishang520/socket.io/v2/socket"
)

type TempSetData struct {
	ProbeId string
	SetTemp float64
}

func main() {
	log.Default().Println("Starting up")
	config := LoadConfig()
	defer config.WriteConfig()
	log.Default().Println("Config loaded")

	os.MkdirAll(config.LogLocation, 0755)
	log.Default().Println("Created log location:", config.LogLocation)

	log.Default().Println("Setting up logger")
	logger := &lumberjack.Logger{
		Filename: config.LogLocation + "bahoa.log",
		MaxSize: 200, // megabytes
		MaxBackups: 3,
		MaxAge: 28, // days
		Compress: true,
	}

	mw := io.MultiWriter(os.Stdout, logger)

	log.SetOutput(mw)
	defer logger.Close()
	log.Default().Println("Logger set up")

	// authMiddleware := func (next http.Handler) http.Handler {
	// 	allowedIPMap := map[string]bool{
	// 		"127.0.0.1": true,
	// 	}
	
	// 	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	// 		address := strings.Split(r.RemoteAddr, ":")[0]
	// 		log.Printf("Request from %v\n", r.RemoteAddr)
	// 		match, _ := regexp.MatchString(`\[::1\]`, r.RemoteAddr)
	// 		if match {
	// 			log.Default().Println(r.RemoteAddr, "authorized IPv6 localhost")
	// 			w.Header().Add("Access-Control-Allow-Origin", "*")
	// 			next.ServeHTTP(w, r)
	// 			return
	// 		}
	// 		if allowedIPMap[address] {
	// 			log.Default().Println(r.RemoteAddr, "authorized IPv4 localhost")
	// 			w.Header().Add("Access-Control-Allow-Origin", "*")
	// 			next.ServeHTTP(w, r)
	// 			return
	// 		}
	// 		if r.Header.Get("Authorization") == "Bearer " + config.LocalAuth {
	// 			log.Default().Println(r.RemoteAddr, "authorized local auth")
	// 			w.Header().Add("Access-Control-Allow-Origin", "*")
	// 			next.ServeHTTP(w, r)
	// 			return
	// 		}
			
	// 		log.Default().Println(r.RemoteAddr, "not authorized")
	// 		http.Error(w, "Forbidden", http.StatusUnauthorized)
	// 	})
	// }

	authMiddleware := func (reqSocket *sio.Socket, next func(err *sio.ExtendedError)) {
		allowedIPMap := map[string]bool{
			"127.0.0.1": true,
		}

		request := reqSocket.Client().Request().Request()
		remoteAddress := request.RemoteAddr

		authHeader:= request.Header.Get("Authorization")
	
		address := strings.Split(remoteAddress, ":")[0]
		log.Printf("Request from %v\n", remoteAddress)
		match, _ := regexp.MatchString(`\[::1\]|172\.\d{1,3}\.\d{1,3}\.\d{1,3}`, remoteAddress)
		if match {
			log.Default().Println(remoteAddress, "authorized IPv6 localhost")
			next(nil)
			return
		}
		if allowedIPMap[address] {
			log.Default().Println(remoteAddress, "authorized IPv4 localhost")
			next(nil)
			return
		}
		if authHeader == "Bearer " + config.LocalAuth {
			log.Default().Println(remoteAddress, "authorized local auth")
			next(nil)
			return
		}
		
		log.Default().Println(remoteAddress, "not authorized")
		next(sio.NewExtendedError("Unauthorized", 401))
	}

	log.Default().Println("Connecting to Redis")
	redis := NewRedis(config.Redis.Host, config.Redis.Port, &config.Redis.Password, 20 * time.Minute)
	log.Default().Println("Connected to Redis")

	log.Default().Println("Creating TimeSeries in Redis")
	redis.CreateTS("probe-0-temp", true)
	redis.CreateTS("probe-1-temp", true)
	redis.CreateTS("probe-2-temp", true)
	redis.CreateTS("probe-3-temp", true)
	redis.CreateTS("probe-0-set-temp", false)
	redis.CreateTS("probe-1-set-temp", false)
	redis.CreateTS("probe-2-set-temp", false)
	redis.CreateTS("probe-3-set-temp", false)
	log.Default().Println("TimeSeries created")

	log.Default().Println("Connecting to wpa_supplicant")
	wifi := NewWifi()
	defer wifi.Close()

	log.Default().Println("Creating to Raspberry Pi robot")
	raspiAdaptor := raspi.NewAdaptor()
	raspiAdaptor.Connect()

	log.Default().Println("Connecting to ADS1115")
	ads1115driver := i2c.NewADS1115Driver(raspiAdaptor, i2c.WithADS1x15BestGainForVoltage(3.3))


	log.Default().Println("Setting up GPIO connection")
	rpio.Open()

	log.Default().Println("Setting heat pin defaults")
	heatPin := rpio.Pin(22)
	heatPin.Output()
	heatPin.Low()
	defer heatPin.Low()
	log.Default().Println("Setting up duty cycle")
	dutyCycle := NewDutyCycle(time.Duration(config.DutyCycle.Period)*time.Second, 0.0, heatPin.High, heatPin.Low)

	log.Default().Println("Setting up PID controller")
	pidInput := 0.0
	pidOutput := 0.0
	pidSetpoint := 0.0
	pidController := NewQuickPID(&pidInput, &pidOutput, &pidSetpoint, config.Pid.ProportionalGain, config.Pid.IntegralGain, config.Pid.DerivativeGain)
	pidController.Initialize()

	tuningPid := false

	// log.Default().Println("Setting up PID controller")
	// pidController := pid.Controller{
	// 	Config: pid.ControllerConfig{
	// 		ProportionalGain: config.Pid.ProportionalGain,
	// 		IntegralGain: config.Pid.IntegralGain,
	// 		DerivativeGain: config.Pid.DerivativeGain,
	// 	},
	// }

	probes := []Probe{}

	work := func() {

		port := "0.0.0.0:3000"

		log.Default().Println("Setting up socket.io server")
		socketOptions := sio.DefaultServerOptions()
		socketOptions.SetCors(&types.Cors{
			Origin: "*",
		})
		socketOptions.SetPath("/socket.io/")

		// router := mux.NewRouter()

		// router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// 	log.Println("Hello World!")
		// 	w.Write([]byte("poopsicles!"))
		// })

		// router.Handle("/socket.io/", socketServer.ServeHandler(socketOptions))

		// muxServer := types.NewServeMux(router)
		httpServer := types.NewWebServer(nil)

		socketServer := sio.NewServer(httpServer, socketOptions)

		socketServer.Use(authMiddleware)

		// router.Use(authMiddleware)

		graphTimeframe := 10 * time.Minute

		log.Default().Println("Setting up socket.io global listeners")
		socketServer.On("connection", func(clients ...any) {
			socket := clients[0].(*sio.Socket)

			socket.Emit("connected", "Connected to server")
			log.Println("Socket", socket.Id(), "from", socket.Client().Request().Request().RemoteAddr, "connected")
			socket.On("joinRoom", func(data ...any) {
				room := data[0].(string)
				log.Default().Println("Adding client", socket.Id(), "to room", room)
				socket.Join(sio.Room(room))

				var callbackFn func([]interface{}, error)
				if fn, ok := data[len(data)-1].(func([]interface{}, error)); ok {
					callbackFn = fn

					callbackFn([]interface{}{ map[string]interface{}{
						"message": "Joined room " + room,
					},
					}, nil)

				} else {
					log.Default().Println("Callback is not a function")
				}

				// for _, room := range rooms {
				// 	log.Default().Println("Adding client", socket.Id(), "to room", room)
				// 	socket.Join([]sio.Room(room.(string)))
				// }
			})

			socket.On("leaveRoom", func(data ...any) {
				room := data[0].(string)
				log.Default().Println("Removing client", socket.Id(), "from room", room)
				socket.Leave(sio.Room(room))

				var callbackFn func([]interface{}, error)
				if fn, ok := data[len(data)-1].(func([]interface{}, error)); ok {
					callbackFn = fn
					
					callbackFn([]interface{}{ map[string]interface{}{
						"message": "Left room " + room,
					},
					}, nil)
				} else {
					log.Default().Println("Callback is not a function")
				}
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

			socket.On("getHistoricalData", func(data ...any) {
				log.Default().Println(data[0])
				if timeframeData, ok := data[0].(map[string]interface{});  ok {
					probeId := timeframeData["probeId"].(string)
					timeframeStart, startErr := time.Parse(time.RFC3339, timeframeData["start"].(string))
					timeFrameEnd, endErr := time.Parse(time.RFC3339, timeframeData["end"].(string))
	
					go func () {
							
						var callbackFn func([]interface{}, error)
						if fn, ok := data[len(data)-1].(func([]interface{}, error)); ok {
							callbackFn = fn
							
							if startErr != nil {
								log.Default().Println("Error parsing timeframes")
								callbackFn(nil, startErr)
							}
							if endErr != nil {
								log.Default().Println("Error parsing timeframes")
								callbackFn(nil, endErr)
							}
							log.Default().Println("start time:", timeframeStart)
							log.Default().Println("end time:", timeFrameEnd)
							historicalData := redis.GetDataRange(probeId, timeframeStart, timeFrameEnd)
							log.Default().Println("Callback is a function!")
							jsonData := []interface{}{historicalData}
							callbackFn(jsonData, nil)
	
						} else {
							log.Default().Println("Callback is not a function")
						}
					}()
				} else {
					return
				}
			})

			socket.On("getGraphData", func(a ...any) {
				if probeId, ok := a[0].(string); ok {
					go func () {
						recentEntries := redis.GetRecentEntries(probeId, graphTimeframe)

						var callbackFn func([]interface{}, error)
						if fn, ok := a[len(a)-1].(func([]interface{}, error)); ok {
							callbackFn = fn

							log.Default().Println("Callback is a function!")
							
							callbackFn([]interface{}{recentEntries}, nil)
						} else {
							log.Default().Println("Callback is not a function")
						}
					}()
				}
			})

			socket.On("getProbe", func(a ...any) {
				if probeId, ok := a[0].(string); ok {
					go func () {
						probeIndex := slices.IndexFunc(probes, func(p Probe) bool { return p.Id == probeId })
						if probeIndex != -1 {
							probe := probes[probeIndex]
							tempK, _ := probe.ReadTempK()
							tempC, _ := probe.ReadTempC()
							tempF, _ := probe.ReadTempF()
							probeData := map[string]interface{}{
								"id": probe.Id,
								"name": probe.Name,
								"tempK": tempK,
								"tempC": tempC,
								"tempF": tempF,
								"setTempK": probe.GetSetTempK(),
								"setTempC": probe.GetSetTempC(),
								"setTempF": probe.GetSetTempF(),
							}
							var callbackFn func([]interface{}, error)
							if fn, ok := a[len(a)-1].(func([]interface{}, error)); ok {
								callbackFn = fn

								log.Default().Println("Callback is a function!")

								callbackFn([]interface{}{probeData}, nil)
							} else {
								log.Default().Println("Callback is not a function")
							}
						} else {
							log.Default().Println("Probe not found")
						}
					}()
				}
			})

			socket.On("getProbes", func(a ...any) {
				go func () {
					probeDataList := []map[string]interface{}{}
					for _, probe := range probes {
						tempK, _ := probe.ReadTempK()
						tempC, _ := probe.ReadTempC()
						tempF, _ := probe.ReadTempF()
						probeData := map[string]interface{}{
							"id": probe.Id,
							"name": probe.Name,
							"tempK": tempK,
							"tempC": tempC,
							"tempF": tempF,
							"setTempK": probe.GetSetTempK(),
							"setTempC": probe.GetSetTempC(),
							"setTempF": probe.GetSetTempF(),
						}
						probeDataList = append(probeDataList, probeData)
					}
					var callbackFn func([]interface{}, error)
					if fn, ok := a[len(a)-1].(func([]interface{}, error)); ok {
						callbackFn = fn

						log.Default().Println("Callback is a function!")

						callbackFn([]interface{}{probeDataList}, nil)
					} else {
						log.Default().Println("Callback is not a function")
					}
				}()
			})

			socket.On("getPID", func(a ...any) {
				pidData := map[string]float64{
					"p": pidController.GetKp(),
					"i": pidController.GetKi(),
					"d": pidController.GetKd(),
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

				pidController.SetTunings(pidData["p"].(float64), pidData["i"].(float64), pidData["d"].(float64))
				config.Pid.ProportionalGain = pidData["p"].(float64)
				config.Pid.IntegralGain = pidData["i"].(float64)
				config.Pid.DerivativeGain = pidData["d"].(float64)
				config.WriteConfig()
				newPidData := map[string]float64{
					"p": pidController.GetKp(),
					"i": pidController.GetKi(),
					"d": pidController.GetKd(),
				}
				socket.Emit("PID", newPidData)
			})

			socket.On("autoTunePID", func(a ...any) {
				tuningData := a[0].(map[string]interface{})
				defer func() {
					if r := recover(); r != nil {
						log.Default().Println("Recovered in f", r)
					}
				}()

				var callbackFn func(map[string]float64, error)
				if fn, ok := a[len(a)-1].(func(map[string]float64, error)); ok {
					callbackFn = fn

					log.Default().Println("Callback is a function!")

					smokerProbeIndex := slices.IndexFunc(probes, func(p Probe) bool { return p.Id == "probe-0" })
					if (smokerProbeIndex == -1) {
						callbackFn(nil, fmt.Errorf("smoker probe not found"))
						return
					}



					if setpoint, ok := tuningData["temperature"].(float64); ok {
						if setpoint > config.Smoker.MaxTempK {
							callbackFn(nil, fmt.Errorf("temperature too high"))
							return
						}
						if setpoint < config.Smoker.MinTempK {
							callbackFn(nil, fmt.Errorf("temperature too low"))
							return
						}

						tuningMethod := ZieglerNicholsPID
						if tuningData["tuningMethod"] != nil {
							if tuningMethodCast, ok := tuningData["tuningMethod"].(TuningMethod); ok {
								tuningMethod = tuningMethodCast
							}
						}

						smokerProbe := probes[smokerProbeIndex]

						sampleTime := dutyCycle.Period
						print := true

						outputStep := 5.0
						hysteresis := 1.0

						go func() {
							tuningPid = true
							pidController.SetMode(Manual)
							defer func() {tuningPid = false}()

							pidController.NewAutoTune(tuningMethod)

							pidController.AutoTune.AutoTuneConfig(outputStep, hysteresis, setpoint, 85.0, pidController.controllerDirection, print, sampleTime)

							pidSetpoint = setpoint

							outKp, outKi, outKd := 0.0, 0.0, 0.0

							preheatCurrentTemp, _ := smokerProbe.ReadTempK()
							for preheatCurrentTemp + 20 < setpoint {
								preheatCurrentTemp, _ = smokerProbe.ReadTempK()
								dutyCycle.SetDutyCyclePercent(100)
								time.Sleep(sampleTime)
							}

							var autoTuneStage AutoTuneStage
							for autoTuneStage != Clr {
								currentTemp, _ := smokerProbe.ReadTempK()

								autoTuneStage = pidController.AutoTune.AutoTuneLoop()

								switch autoTuneStage {
									case AutoTune:
										pidInput = currentTemp
										dutyCycle.SetDutyCyclePercent(gobot.Rescale(pidOutput, 0, 255, 0, 100))
										socket.Emit("pidTune", "AutoTune")
									case Tunings:
										pidController.AutoTune.SetAutoTuneConstants(&outKp, &outKi, &outKd)
										pidController.SetMode(Automatic)
										socket.Emit("pidTune", "Tunings")
									case Clr:
										pidController.ClearAutoTune()
										socket.Emit("pidTune", "Clr")
								}
								time.Sleep(sampleTime)
							}

							callbackFn(map[string]float64{
								"p": outKp,
								"i": outKi,
								"d": outKd,
							}, nil)
						}()
					} else {
						callbackFn(nil, fmt.Errorf("temperature not provided"))
						return
					}
				} else {
					log.Default().Println("Callback is not a function")
				}
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

			socket.On("setProbeName", func(data ...any) {
				nameData := data[0].(map[string]interface{})
				probeIndex := slices.IndexFunc(probes, func(p Probe) bool { return p.Id == nameData["probeId"].(string) })
				if probeIndex != -1 {
					probes[probeIndex].Name = nameData["name"].(string)
				}
			})
		})

		log.Default().Println("Starting socket.io server")
		httpServer.Listen(port, nil)
		log.Default().Printf("Listening on http://%v\n", port)
		// log.Fatal(http.ListenAndServe(port, handlers.CORS(handlers.AllowedOrigins([]string{"*"}))(router)))

		probeDataList := []map[string]interface{}{}

		log.Default().Println("Starting wifi polling loop")
		gobot.Every(30*time.Second, func() {
			if socketServer.ListenerCount("wifi") == 0 {
				return
			}
			go func() {
				currentNetwork, err := wifi.GetCurrentNetwork()
				if err != nil {
					currentNetwork = Wlan{}
				}
				log.Default().Println(currentNetwork)
				socketServer.To("pi").Emit("wifi", currentNetwork)
			}()
		})

		gobot.Every(1*time.Second, func() {
			for _, probe := range probes {
				socketServer.To(sio.Room(probe.Id + "-graph")).Emit("graphData", redis.GetRecentEntries(probe.Id, graphTimeframe))
			}
		})

		// PID loop
		gobot.Every(dutyCycle.Period, func() {
			if !tuningPid {
				smokerProbeIndex := slices.IndexFunc(probes, func(p Probe) bool { return p.Id == "probe-0" })
				if smokerProbeIndex != -1 {
					setTemp := probes[smokerProbeIndex].GetSetTempK()
					temp, err := probes[smokerProbeIndex].ReadTempK()
					if err != nil {
						return
					}
					if setTemp > temp + 20 {
						dutyCycle.SetDutyCyclePercent(100)
					} else if setTemp > temp {
						if pidController.GetMode() != Automatic {
							pidController.SetMode(Automatic)
						}
						pidInput = temp
						pidSetpoint = setTemp
						pidController.Compute()
						dutyCycle.SetDutyCyclePercent(gobot.Rescale(pidOutput, 0, 255, 0, 100))
					} else {
						if pidController.GetMode() != Manual {
							pidController.SetMode(Manual)
						}
						dutyCycle.SetDutyCyclePercent(0)
					}
				}
			}
		})
		

		adc := NewADC(3.3, ads1115driver)
		probeChannels := []int{0, 1, 2, 3}
		procTime := time.Duration(config.ProcessTime)*time.Millisecond
		gobot.Every(procTime, func() {
			probeDataList = []map[string]interface{}{}
			for i, channel := range probeChannels {
				index := slices.IndexFunc(probes, func(p Probe) bool {
					return p.Id == fmt.Sprintf("probe-%v", i)
				})
				if index == -1 {
					if channel == 0 {
						probe := NewProbe(fmt.Sprintf("probe-%v", i), "smoker", 0x48, channel, 100000, adc, 50)
						
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
						socketServer.To("pi", "web").Emit("probeDisconnected", fmt.Sprintf("probe-%v", i))
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

					avgTempK := redis.GetAvg(probe.Id + "-temp")
					probeData := map[string]interface{}{
						"id": probe.Id,
						"name": probe.Name,
						"tempK": tempK,
						"tempC": tempC,
						"tempF": tempF,
						"setTempK": probe.GetSetTempK(),
						"setTempC": probe.GetSetTempC(),
						"setTempF": probe.GetSetTempF(),
						"avgTempK": avgTempK,
						"avgTempC": avgTempK - 273.15,
						"avgTempF": (avgTempK - 273.15) * 9 / 5 + 32,
					}
					probeDataList = append(probeDataList, probeData)
					socketServer.In(sio.Room(probe.Id)).Emit("probe", probeData)
					// socketServer.In("pi", "web").Emit("probe", probeData)
					// socketServer.In("pi", "web").Emit("probeRecords", redis.GetRecentEntries(probe.Id, graphTimeframe))
				}
			}
			socketServer.To("pi", "web").Emit("probes", probeDataList)
			// if len(probes) > 0 {
			// 	smokerProbeIndex := slices.IndexFunc(probes, func(p Probe) bool { return p.Id == "probe-0" })
			// 	if smokerProbeIndex != -1 && probes[smokerProbeIndex].ReadConnected() {
			// 		smokerProbe := probes[smokerProbeIndex]
			// 		currentTemp, _ := smokerProbe.ReadTempK()
			// 		setTemp := smokerProbe.GetSetTempK()
			// 		if setTemp == 0 {
			// 			// log.Default().Println("No set temp. Turning off heat.")
			// 			dutyCycle.SetDutyCyclePercent(0)
			// 		} else if (setTemp - currentTemp) > 10 {
			// 			// log.Default().Println("Smoker low enough that PID doesn't matter. Turning on heat.")
			// 			dutyCycle.SetDutyCyclePercent(100)
			// 		} else {
			// 			// log.Default().Println("Smoker close enough to PID. Switching to PID control.")
			// 			pidController.Update(pid.ControllerInput{
			// 				ReferenceSignal: setTemp,
			// 				ActualSignal: currentTemp,
			// 				SamplingInterval: procTime,
			// 			})
			// 			pidOutput := pidController.State.ControlSignal
			// 			// log.Default().Printf("PID Error: %v\n", pidController.State.ControlError)
			// 			// log.Default().Printf("PID Output: %v\n", pidOutput)
			// 			scaledOutput := 0.0
			// 			if pidOutput > 0 {
			// 				scaledOutput = 0 * (1 - pidOutput / setTemp) + 100 * (pidOutput / setTemp)
			// 			}
			// 			// log.Default().Printf("Scaled Output: %v\n", scaledOutput)
			// 			dutyCycle.SetDutyCyclePercent(scaledOutput)
			// 			// log.Default().Printf("Duty Cycle: %v\n", dutyCycle.GetDutyCycle() / time.Millisecond)
			// 		}
			// 	}
			// }
		})
	}

	cleanup := func() {
		logger.Close()
		wifi.Close()
		heatPin.Low()
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<- c
		log.Default().Printf("cleaning up")
		cleanup()
		os.Exit(1)
	}()

	log.Default().Println("Configuring robot")
	robot := gobot.NewRobot("ads1115Bot",
		[]gobot.Connection{raspiAdaptor},
		[]gobot.Device{ads1115driver},
		work,
	)

	log.Default().Println("Starting robot")
	robot.Start()

}

func roundFloat(val float64, precision uint) float64 {
    ratio := math.Pow(10, float64(precision))
    return math.Round(val*ratio) / ratio
}