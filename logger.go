package main

import "log"

// logf logs with a component prefix for easy grep-ability.
func logf(component, format string, args ...interface{}) {
	log.Printf("[%s] "+format, append([]interface{}{component}, args...)...)
}
