package main

import (
	"bufio"
	"log"
	"strings"
	"syscall"
)

func safeExitOnError(args ...interface{}) {
	if r := recover(); r != nil {
		log.Printf("Resuming after error: %v\n", r)
		log.Printf(args[0].(string), args[1:]...)
	}
}

func getMapData(from interface{}, path string) (ret interface{}, ok bool) {
	components := strings.Split(path, ".")
	var current = from
	for _, component := range components {
		if current, ok = current.(map[string]interface{})[component]; !ok {
			return nil, ok
		}
	}

	return current, true
}

func getMapDataStr(from interface{}, path string) (ret string, ok bool) {
	if data, ok := getMapData(from, path); ok {
		str, ok := data.(string)
		return str, ok
	}

	return "", ok
}

func forEachRecursive(prefixes string, from interface{}, routine func(key string, val string)) {
	if str, ok := from.(string); ok {
		routine(prefixes, str)
	} else if mp, ok := from.(map[interface{}]interface{}); ok {
		for key, value := range mp {
			newkey := key.(string)
			if len(prefixes) > 0 {
				newkey = prefixes + "." + newkey
			}
			forEachRecursive(newkey, value, routine)
		}
	}
}

func fdSet(p *syscall.FdSet, i int) {
	p.Bits[i/64] |= 1 << uint(i) % 64
}

func fdIsSet(p *syscall.FdSet, i int) bool {
	return (p.Bits[i/64] & (1 << uint(i) % 64)) != 0
}

func fdZero(p *syscall.FdSet) {
	for i := range p.Bits {
		p.Bits[i] = 0
	}
}

func readField(in *bufio.Reader, sep byte) string {
	val, err := in.ReadString(sep)
	if err != nil {
		panic(err)
	}
	return val[:len(val)-1]
}
