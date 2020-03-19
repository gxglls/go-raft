package raft

import (
	"log"
)

type LogLevel int


const (
	INFO LogLevel = 1
	DEBUG LogLevel = 2
)

func Logf (l LogLevel, CfgLogLevel LogLevel, args ... interface{}) {
	if CfgLogLevel < l {
		return
	}
	log.SetFlags( log.Lshortfile | log.Ltime | log.Lmicroseconds)
	log.Println(args...)
}
