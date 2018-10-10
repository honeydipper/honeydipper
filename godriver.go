package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

// GoDriver : a driver type that runs a golang program process
type GoDriver struct {
	Driver
	Package    string
	Executable string
	Arguments  []string
}

// NewGoDriver : create a driver object to run the golang program process
func NewGoDriver(data map[string]interface{}) GoDriver {
	pack, ok := data["Package"].(string)
	if !ok {
		log.Panic("Package is not sepcified in driver")
	}

	cmd, ok := data["Executable"].(string)
	if !ok {
		packParts := strings.Split(pack, "/")
		cmd = packParts[len(packParts)-1]
	}

	args, ok := data["Arguments"].([]string)
	if !ok {
		args = []string{}
	}

	driver := GoDriver{
		Package:    pack,
		Executable: cmd,
		Arguments:  args,
	}
	driver.Type = "go"
	return driver
}

func (g *GoDriver) start(service string, runtime *DriverRuntime) {
	check := exec.Command("go", "list", g.Package)
	if err := check.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			install := exec.Command("go", "get", g.Package)
			if err := install.Run(); err != nil {
				log.Panic("Unable to install the go package for driver")
			}
		}
	}

	args := append([]string{service}, g.Arguments...)
	run := exec.Command(g.Executable, args...)
	if input, err := run.StdoutPipe(); err != nil {
		log.Panicf("Unable to link to driver stdout %v", err)
	} else {
		runtime.input = &(input)
	}
	if output, err := run.StdinPipe(); err != nil {
		log.Panicf("Unable to link to driver stdin %v", err)
	} else {
		runtime.output = &(output)
	}
	run.Stderr = os.Stderr
	if err := run.Start(); err != nil {
		log.Panicf("Failed to start driver %v", err)
	}
	runtime.driver = &g.Driver

	forEachRecursive([]interface{}{}, *runtime.data, func(keyObjs []interface{}, val string) {
		key := []string{}
		for _, obj := range keyObjs {
			key = append(key, fmt.Sprintf("%v", obj))
		}
		log.Printf("sending to driver option:%s\\n%s\n", strings.Join(key, "."), val)
		fmt.Fprintf((*runtime.output).(io.Writer), "option:%s\n%s\n", strings.Join(key, "."), val)
	})

	fmt.Fprintf((*runtime.output).(io.Writer), "action:go\n")
	retry := 3
	for ; retry > 0; retry-- {
		fmt.Fprintf((*runtime.output).(io.Writer), "action:ping\n")
		started := false
		alive := false
		ch := make(chan int)
		go func() {
			fmt.Fscanf((*runtime.input).(io.Reader), "signal:pong{started:%t,alive:%t}\n", &started, &alive)
			ch <- 1
		}()

		select {
		case <-ch:
			log.Printf("pong received")
		case <-time.After(3 * time.Second):
			log.Printf("pong not received after 3 seconds")
		}
		if alive {
			log.Printf("driver is alive")
			break
		}
		time.Sleep(time.Second)
	}

	if retry == 0 {
		fmt.Fprintf((*runtime.output).(io.Writer), "action:quit\n")
		log.Panic("driver is not alive")
	}
}
