package main

import (
	"log"
	wifi "github.com/theojulienne/go-wireless"
)

type Wifi struct {
	client *wifi.Client
	lastWlan Wlan
}

func NewWifi() *Wifi {
	client, err := wifi.NewClient("wlan0")
	if err != nil {
		log.Println(err)
		return nil
	}
	return &Wifi{
		client: client,
		lastWlan: Wlan{},
	}
}

func (w *Wifi) Close() {
	if (w.client != nil) {
		w.client.Close()
	}
}

func (w *Wifi) GetAps() ([]wifi.AP, error) {
	log.Default().Println("Scanning for networks")

	aps, err := w.client.Scan()
	if err != nil {
		log.Default().Println(err)
		return nil, err
	}
	return aps, nil
}

type Wlan struct {
	ID 				string
	SSID 			string
	BSSID 			string
	ESSID 			string
	Mode			string
	KeyManagement	string
	WpaState		string
	IPAddress		string
	Address			string
	UUID			string
	GroupCipher		string
	PairwiseCipher	string
	RSSI			int
	Signal			int
	Frequency		int
}

func (w *Wifi) GetCurrentNetwork() (Wlan, error) {
	aps, _ := w.GetAps()
	status, err := w.client.Status()
	extendedDetail, _ := wifi.APs(aps).FindBySSID(status.SSID)
	if err != nil {
		log.Default().Println(err)
		return w.lastWlan, err
	}
	network := Wlan{
		ID: status.ID,
		SSID: status.SSID,
		BSSID: status.BSSID,
		ESSID: extendedDetail.ESSID,
		Mode: status.Mode,
		KeyManagement: status.KeyManagement,
		WpaState: status.WpaState,
		IPAddress: status.IPAddress,
		Address: status.Address,
		UUID: status.UUID,
		GroupCipher: status.GroupCipher,
		PairwiseCipher: status.PairwiseCipher,
		RSSI: extendedDetail.RSSI,
		Signal: extendedDetail.Signal,
		Frequency: extendedDetail.Frequency,
	}
	w.lastWlan = network
	return network, nil
}

func (w *Wifi) GetKnownNetworks() ([]wifi.Network, error) {
	networks, err := w.client.Networks()
	if err != nil {
		log.Default().Println(err)
		return nil, err
	}
	
	return networks, nil
}