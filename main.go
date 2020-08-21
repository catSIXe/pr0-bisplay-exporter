package main

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"reflect"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/catSIXe/pr0-bisplay/settings"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const _SyncURI = "https://pr0gramm.com/api/user/sync?offset=-1"
const _ProfileURI = "https://pr0gramm.com/api/profile/info?flags=1&name="
const _GetItemsURI = "https://pr0gramm.com/api/items/get?flags=1"

// Pr0Stats is the struct the ESP32 also has a copy off for the protocol
type Pr0Stats struct {
	head           byte
	benis          int32
	deltaBenis     int32
	unreadMessages byte
	maxHochladeID  uint32
}

var (
	stats *Pr0Stats
	conf  *settings.App

	benisMetric = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "pr0_benis",
		Help: "Benis Size",
	})
)

func pr0APIcall(url string, decoderOutput *map[string]interface{}, meCookie *http.Cookie) (err error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return
	}

	req.AddCookie(meCookie)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		err = errors.New(url +
			"\nresp.StatusCode: " + strconv.Itoa(resp.StatusCode))
		return
	}

	if err = json.NewDecoder(resp.Body).Decode(&decoderOutput); err != nil {
		panic(err)
	}
	return nil
}

// MaxU32 is math:max for unsigned 32bit ints
func MaxU32(x, y uint32) uint32 {
	if x > y {
		return x
	}
	return y
}

// fetches pr0gramm info
func pr0Fetch() (err error) {
	var (
		meCookie         *http.Cookie
		syncResponse     map[string]interface{}
		profileResponse  map[string]interface{}
		getItemsResponse map[string]interface{}
		unreadMessages   byte
		alterBenis       int32 = 0
	)
	meCookie = &http.Cookie{Name: "me", Value: conf.Cookie}
	for {
		// ungelese Nachrichten
		if err = pr0APIcall(_SyncURI, &syncResponse, meCookie); err != nil {
			return err
		}
		unreadMessages = 0
		for _, v := range syncResponse["inbox"].(map[string]interface{}) {
			_ = v
			if reflect.TypeOf(v).String() == "int" {
				unreadMessages += byte(v.(int))
			}
		}
		stats.unreadMessages = unreadMessages
		// Benis Score
		if err = pr0APIcall(_ProfileURI+conf.Username, &profileResponse, meCookie); err != nil {
			return err
		}
		stats.benis = int32(profileResponse["user"].(map[string]interface{})["score"].(float64))
		stats.deltaBenis = stats.benis - alterBenis
		alterBenis = stats.benis
		benisMetric.Set(float64(stats.benis))

		// Hochlade ID
		if err = pr0APIcall(_GetItemsURI, &getItemsResponse, meCookie); err != nil {
			return err
		}
		for _, v := range getItemsResponse["items"].([]interface{}) {
			subInt := uint32(v.(map[string]interface{})["id"].(float64))
			stats.maxHochladeID = MaxU32(subInt, stats.maxHochladeID)
		}

		// das pr0 bei Nacht am Morgen ruhen lassen
		time.Sleep(time.Second * 60)
	}
}

// sends udp data
func pr0Transmit() (err error) {
	var (
		ServerAddr       *net.UDPAddr
		Conn             *net.UDPConn
		settingsRegister byte
	)
	stats = &Pr0Stats{head: 0x01}
	byteArray := make([]byte, (1+4+4+1+4)+(1)) // +1 Byte extra für Einstellungsregister
	if ServerAddr, err = net.ResolveUDPAddr("udp", conf.TargetIP); err != nil {
		return err
	}
	if Conn, err = net.DialUDP("udp", nil, ServerAddr); err != nil {
		return err
	}

	settingsRegister = conf.SettingNotificationFlash
	settingsRegister |= conf.SettingOnlyBenis << 1
	settingsRegister |= conf.SettingHideTrend << 2
	settingsRegister |= conf.SettingHideHochladID << 3
	settingsRegister |= conf.SettingHideNotificationCount << 4
	settingsRegister |= conf.Setting5 << 5
	settingsRegister |= conf.Setting6 << 6
	settingsRegister |= conf.Setting7 << 7

	defer Conn.Close()
	for {
		byteArray[0] = stats.head
		binary.LittleEndian.PutUint32(byteArray[1:], uint32(stats.benis))
		binary.LittleEndian.PutUint32(byteArray[5:], uint32(stats.deltaBenis))
		byteArray[1+4+4] = stats.unreadMessages
		binary.LittleEndian.PutUint32(byteArray[10:], uint32(stats.maxHochladeID))
		byteArray[1+4+4+1+4] = settingsRegister
		// fmt.Println(byteArray)
		if _, err := Conn.Write(byteArray); err != nil {
			fmt.Println(err)
		}
		time.Sleep(time.Second * 5)
	}
}

func main() {
	var err error
	if conf, err = settings.LoadSettings(); err != nil {
		fmt.Errorf("Bitte pruefe deine Einstellungen in der .env Datei oder den Umgebungsvariabeln")
		return
	}

	wg := new(sync.WaitGroup)
	wg.Add(2 + 1)

	go func() {
		if err = pr0Transmit(); err != nil {
			panic(err)
		}
		wg.Done()
	}()
	go func() {
		if err = pr0Fetch(); err != nil {
			panic(err)
		}
		wg.Done()
	}()

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		log.Fatal(http.ListenAndServe(":1337", nil))
		wg.Done()
	}()

	wg.Wait()
}
