package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gofrs/flock"
	bytesize "github.com/inhies/go-bytesize"
	"github.com/pbnjay/memory"
	"github.com/spf13/cobra"
)

var config = "/tmp/resourcer.conf"
var dir = "/tmp/resourcer"
var mem = "1GB"
var cores = 1

var initConfig = "/tmp/resourcer.conf"
var initMem = ""
var initCores = 0

type Request struct {
	Memory uint64 `json:"memory"`
	Cores  int    `json:"cores"`
}

func GetSummary(dir string) (Request, error) {
	m, err := filepath.Glob(filepath.Join(dir, "*.req"))
	if err != nil {
		return Request{}, err
	}
	o := Request{}
	for _, i := range m {
		pidStr := strings.TrimSuffix(filepath.Base(i), ".req")
		pid, err := strconv.Atoi(pidStr)
		if err == nil {
			if CheckProcess(pid) {
				txt, err := ioutil.ReadFile(i)
				if err == nil {
					rec := Request{}
					json.Unmarshal(txt, &rec)
					o.Cores += rec.Cores
					o.Memory += rec.Memory
				}
			} else {
				os.Remove(i)
			}
		}
	}
	log.Printf("Total requests: %d %d", o.Cores, o.Memory)
	return o, nil
}

func CheckProcess(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	} else {
		err := process.Signal(syscall.Signal(0))
		if err != nil {
			return false
		}
	}
	return true
}

func RequestFileName(dir string) string {
	fileName := fmt.Sprintf("%d.req", os.Getpid())
	return filepath.Join(dir, fileName)
}

func MakeRequest(dir string, req Request, max Request) (bool, error) {
	lockPath := filepath.Join(dir, "lockfile")
	fileLock := flock.New(lockPath)

	lockCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	locked, err := fileLock.TryLockContext(lockCtx, 678*time.Millisecond)
	if err != nil {
		return false, err
	}
	if locked {
		defer fileLock.Unlock()
		sum, err := GetSummary(dir)
		if err == nil {
			if sum.Cores+req.Cores <= max.Cores && sum.Memory+req.Memory <= max.Memory {
				d, _ := json.Marshal(req)
				os.WriteFile(RequestFileName(dir), d, 0600)
				log.Printf("Allocating with %d and %d left", max.Cores-sum.Cores-req.Cores, max.Memory-sum.Memory-req.Memory)
				return true, nil
			} else {
				//log.Printf("Waiting for resources: %d > %d and %d > %d",
				//	sum.Cores+req.Cores, max.Cores,
				//	sum.Memory+req.Memory, max.Memory)
			}
		}
	}
	return false, nil
}

func ClearRequest(dir string) {
	os.Remove(RequestFileName(dir))
}

func GetDefaultLimits() Request {
	mem := uint64(float64(memory.TotalMemory()) * 0.9)
	cores := runtime.NumCPU()
	return Request{Memory: mem, Cores: cores}
}

var runCmd = &cobra.Command{
	Use: "run",
	RunE: func(cmd *cobra.Command, args []string) error {

		if _, err := os.Stat(config); os.IsNotExist(err) {
			defaults := GetDefaultLimits()
			json, _ := json.Marshal(defaults)
			os.WriteFile(config, json, 0600)
		}

		configTxt, err := os.ReadFile(config)
		if err != nil {
			os.Exit(1)
		}
		max := Request{}
		err = json.Unmarshal(configTxt, &max)
		if err != nil {
			log.Fatal(err)
			os.Exit(1)
		}
		memSize, err := bytesize.Parse(mem)
		if err != nil {
			return err
		}

		if memSize > bytesize.ByteSize(max.Memory) || cores > max.Cores {
			return fmt.Errorf("Requesting more then avalible")
		}

		if _, err := os.Stat(dir); os.IsNotExist(err) {
			os.Mkdir(dir, 0700)
		}

		var sleepTime time.Duration = 1 * time.Second
		req := Request{Memory: uint64(memSize), Cores: cores}
		for ok := false; !ok; ok, err = MakeRequest(dir, req, max) {
			log.Printf("Making Request")
			if err != nil {
				log.Printf("Error: %s", err)
				return err
			}
			time.Sleep(sleepTime)
			sleepTime += time.Second
			if sleepTime > time.Second*10 {
				sleepTime = time.Second * 10
			}
		}
		defer ClearRequest(dir)
		//log.Printf("Starting: %s", args)
		cmdLine := exec.Command(args[0], args[1:]...)
		cmdLine.Stderr = os.Stderr
		cmdLine.Stdout = os.Stdout
		err = cmdLine.Start()
		if err != nil {
			return err
		}
		if err := cmdLine.Wait(); err != nil {
			return err
		}
		return nil
	},
}

var initCmd = &cobra.Command{
	Use: "init",
	RunE: func(cmd *cobra.Command, args []string) error {
		defaults := GetDefaultLimits()
		var memSize uint64
		var coreCount int

		if initMem == "" {
			memSize = defaults.Memory
		} else {
			ms, err := bytesize.Parse(initMem)
			if err != nil {
				return err
			}
			memSize = uint64(ms)
		}
		if initCores == 0 {
			coreCount = defaults.Cores
		} else {
			coreCount = initCores
		}
		configReq := Request{Memory: memSize, Cores: coreCount}
		json, _ := json.Marshal(configReq)
		os.WriteFile(config, json, 0600)
		return nil
	},
}

func main() {
	runCmd.Flags().StringVarP(&config, "config", "c", config, "Config file path")
	runCmd.Flags().StringVarP(&dir, "dir", "d", dir, "Working directory")
	runCmd.Flags().StringVarP(&mem, "mem", "m", mem, "Memory requested")
	runCmd.Flags().IntVarP(&cores, "cores", "n", cores, "Cores requested")

	initCmd.Flags().StringVarP(&initConfig, "config", "c", initConfig, "Config file path")
	initCmd.Flags().StringVarP(&initMem, "mem", "m", initMem, "Memory Avalible")
	initCmd.Flags().IntVarP(&initCores, "cores", "n", initCores, "Cores Avalible")

	RootCmd := &cobra.Command{
		Use: "resourcer",
	}
	RootCmd.AddCommand(runCmd)
	RootCmd.AddCommand(initCmd)
	if err := RootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
