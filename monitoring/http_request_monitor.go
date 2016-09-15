package monitoring

import (
	"fmt"
	"expvar"
	"net/http"
	"strconv"
)

type HttpRequestMonitor struct {
	baseName                string
	statusName              string
	handlerRequestCountMap  *expvar.Map
	handlerRequestStatusMap *expvar.Map
}

const requestMapSuffix = "http_requests_by_handler"
const statusMapSuffix = "http_status_by_handler"

func NewHttpMonitor(application, component string) *HttpRequestMonitor {
	name := fmt.Sprintf("%s-%s-%s", application, component, requestMapSuffix)
	statusName := fmt.Sprintf("%s-%s-%s", application, component, statusMapSuffix)
	return &HttpRequestMonitor{baseName:name, statusName: statusName, handlerRequestCountMap:expvar.NewMap(name), handlerRequestStatusMap:expvar.NewMap(statusName)}
}

// Publish registers our maps with expvar. This will panic if called more than once for a
// particular combination of application and component.
func (h HttpRequestMonitor) Publish() {
	expvar.Publish(h.baseName, h.handlerRequestCountMap)
	expvar.Publish(h.statusName, h.handlerRequestStatusMap)
}

func (h HttpRequestMonitor) LogRequestStarted(handlerName string) {
	// One more request to this handler (expvar uses atomic counters)
	h.handlerRequestCountMap.Add(handlerName, 1)
}

func (h HttpRequestMonitor) LogRequestCompleted(handler string, status int, err error) {
	// Special case for the handler blowing up before a status was set
	if err != nil && status == 0 {
		h.handlerRequestStatusMap.Add(strconv.Itoa(http.StatusInternalServerError), 1)
	} else {
		// Record the status
		h.handlerRequestStatusMap.Add(strconv.Itoa(status), 1)
	}
}