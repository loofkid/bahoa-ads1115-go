package main

import (
	"fmt"
	"log"
	"math"
	"net/http"
	"slices"
	"strings"
	"time"
	"encoding/json"

	"github.com/gorilla/mux"

	"gobot.io/x/gobot"
	"gobot.io/x/gobot/drivers/i2c"
	"gobot.io/x/gobot/platforms/raspi"

	sio "github.com/zishang520/socket.io/v2/socket"
)

func main() {
	raspiAdaptor := raspi.NewAdaptor()

	ads1115driver := i2c.NewADS1115Driver(raspiAdaptor)

	work := func() {
		port := "0.0.0.0:3000"

		socketServer := sio.NewServer(nil, nil)

		router := mux.NewRouter()

		router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			log.Println("Hello World!")
			w.Write([]byte("poopsicles!"))
		})

		// router.HandleFunc("/socket.io", func(w http.ResponseWriter, r *http.Request) {
		// 	log.Println("Socket.io connection")
		// 	socketServer.ServeHandler(nil).ServeHTTP(w, r)
		// })
		router.Handle("/socket.io/", socketServer.ServeHandler(nil))

		socketServer.On("connection", func(clients ...any) {
			socket := clients[0].(*sio.Socket)

			socket.Emit("connected", "Connected to server")
			log.Printf("Socket %v connected\n", clients)
			socket.Join("probes")
		})

		router.Use(authMiddleware)

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
					probeJson, _ := json.Marshal(map[string]interface{}{
						"id": probe.Id,
						"name": probe.Name,
						"tempK": roundFloat(tempK, 2),
						"tempC": roundFloat(tempC, 2),
						"tempF": roundFloat(tempF, 2),
					})
					socketServer.In("probes").Emit("probe", probeJson)
				}
			}
		})

		log.Printf("Listening on http://%v\n", port)
		log.Fatal(http.ListenAndServe(port, router))
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
		log.Printf("Request from %v\n", address)
		if (allowedIPMap[address]) {
			next.ServeHTTP(w, r)
			return
		}

		http.Error(w, "Forbidden", http.StatusUnauthorized)
	})
}