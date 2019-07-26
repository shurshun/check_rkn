package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/zmap/go-iptree/iptree"
	"github.com/urfave/cli"
	"io"
	// "go.uber.org/zap"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"
	"log"
)

var (
	Version = "1.0.0"
	cliApp = cli.NewApp()
)

func init() {
	cliApp.Version = Version
	cliApp.Usage = "Check IP addresses for blocking by RKN"

	cliApp.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "listen-addr",
			Value:  ":8020",
			Usage:  "HTTP Server listen address",
			EnvVar: "LISTEN_ADDRESS",
		},
		cli.StringFlag{
			Name:   "dump-url",
			Value:  "https://github.com/zapret-info/z-i/raw/master/dump.csv",
			Usage:  "RKN db url",
			EnvVar: "DUMP_URL",
		},
		cli.StringFlag{
			Name:   "dump-dir",
			Value:  "/db",
			Usage:  "Directory to place db",
			EnvVar: "DUMP_DIR",
		},
		cli.IntFlag{
			Name:   "dump-download-timeout",
			Value:  30,
			Usage:  "Dump download timeout (sec)",
			EnvVar: "DUMP_DOWNLOAD_TIMEOUT",
		},
		cli.IntFlag{
			Name:   "dump-download-retry",
			Value:  30,
			Usage:  "Dump download retry interval (sec)",
			EnvVar: "DUMP_DOWNLOAD_RETRY",
		},
		cli.IntFlag{
			Name:   "dump-download-interval",
			Value:  30,
			Usage:  "Dump download interval (min)",
			EnvVar: "DUMP_DOWNLOAD_INTERVAL",
		},
	}

}


func RknChecker(cliContext *cli.Context) error {
	dumpDir := cliContext.String("dump-dir")
	dumpUrl := cliContext.String("dump-url")
	downloadRetryInterval := time.Duration(cliContext.Int("dump-download-retry")) * time.Second
	dumpUpdateInterval := time.Duration(cliContext.Int("dump-download-interval")) * time.Minute
	dumpDownloadTimeout := cliContext.Int("dump-download-timeout")

	lock := sync.Mutex{}

	currentDump := path.Join(dumpDir, "dump.current")
	freshDump := path.Join(dumpDir, "dump.fresh")

	for {
		err := downloadDump(dumpUrl, currentDump, dumpDownloadTimeout)
		if err == nil {
			break
		}
		log.Println(err, "retry after", downloadRetryInterval)
		time.Sleep(downloadRetryInterval)
	}
	db, err := loadDump(currentDump)
	if err != nil {
		log.Fatalln(err)
	}

	go func() {
		ticker := time.NewTicker(dumpUpdateInterval).C
		for range ticker {
			log.Println("updating db")
			err := downloadDump(dumpUrl, freshDump, dumpDownloadTimeout)
			if err != nil {
				log.Println(err)
				continue
			}
			newDb, err := loadDump(freshDump)
			if err != nil {
				log.Println(err)
				continue
			}
			lock.Lock()
			db = newDb
			if err = os.Rename(freshDump, currentDump); err != nil {
				log.Println(err)
			}
			log.Println("db updated")
			lock.Unlock()
		}
	}()

	http.HandleFunc("/v1/rkn/check", func(w http.ResponseWriter, r *http.Request) {
		lock.Lock()
		defer lock.Unlock()

		ips := []string{}

		if err := json.NewDecoder(r.Body).Decode(&ips); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if len(ips) < 1 {
			http.Error(w, "empty ip list", http.StatusBadRequest)
			return
		}
		res := make(map[string]bool, len(ips))

		for _, ip := range ips {
			v, ok, err := db.GetByString(ip)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			res[ip] = false
			if !ok {
				continue
			}
			if flag, _ := v.(int); flag == 1 {
				res[ip] = true
			}
		}
		if err := json.NewEncoder(w).Encode(res); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})

	http.HandleFunc("/_liveness", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "OK")
	})

	http.HandleFunc("/_readiness", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "OK")
	})

	log.Println("listening on", cliContext.String("listen-addr"))
	log.Fatal(http.ListenAndServe(cliContext.String("listen-addr"), nil))

	return nil
}

func main() {
	cliApp.Action = RknChecker
	err := cliApp.Run(os.Args)

	if err != nil {
		log.Fatal(err)
	}
}


func loadDump(path string) (*iptree.IPTree, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	t := iptree.New()
	t.AddByString("0.0.0.0/0", 0)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), ";")
		if len(fields) < 2 {
			continue
		}
		for _, ip := range strings.Split(fields[0], "|") {
			t.AddByString(strings.TrimSpace(ip), 1)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return t, err
}

func downloadDump(url, path string, timeout int) error {
	log.Printf("downloading dump from %s to %s", url, path)
	client := http.Client{Timeout: time.Duration(timeout) * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		log.Printf("file %s exists, removing it", path)
		if err = os.Remove(path); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf(resp.Status)
	}
	defer resp.Body.Close()
	if _, err = io.Copy(f, resp.Body); err != nil {
		return err
	}
	log.Println("dump downloaded")
	return nil
}