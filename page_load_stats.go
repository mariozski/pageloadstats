package pageloadstats

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"time"

	phantomjs "github.com/urturn/go-phantomjs" // exported package is phantomjs
)

var err error

// PageMeasurements is a top level structure to keep measurement data.
type PageMeasurements struct {
	LoadTime         int64                         `json:"loadTime"`
	Responses        map[int32]RequestMeasurements `json:"responses"`
	LoadTimeDuration time.Duration
	ThumbnailFile    string
}

// RequestMeasurements is a structure containing data for each child
// request of main request.
type RequestMeasurements struct {
	StartTime           time.Time `json:"startTime"`
	EndTime             time.Time `json:"endTime"`
	RunningTime         int64     `json:"runningTime"`
	RunningTimeDuration time.Duration
	Status              int32  `json:"status"`
	URL                 string `json:"url"`
}

// PageLoadStats defines operations for ivoking and issuing
// commands to worker processes.
type PageLoadStats interface {
	GetMeasurements(string, int, string) (*PageMeasurements, error)
	Close()
}

// New creates a new instance of object implementing
// LoadTimer interface that can be used to get
// measurements of load time of web page.
func New(poolSize int) PageLoadStats {
	workers := &workersPool{size: poolSize, used: make([]bool, poolSize), workers: make([]*phantomjs.Phantom, poolSize)}

	for i := 0; i < workers.size; i++ {
		(*workers).workers[i], err = phantomjs.Start()
		if err != nil {
			panic(err)
		}
	}

	return workers
}

func getJsFunc(url string, thumbnailFile string) string {
	return fmt.Sprintf(`
		function(done) {
			var page = require('webpage').create(),
				system = require('system'),
				address = %q,
				thumbnailFile = %q,
				loadTime;
	
			var diagnosticData = { responses: {} };
			page.onResourceRequested = function(request) {
				diagnosticData.responses[request.id] = {startTime: request.time, url: request.url};
			};
			page.onResourceReceived = function(response) {
				var responseData = diagnosticData.responses[response.id];
				if (responseData) {
					responseData.status = response.status;
					responseData.endTime = response.time;
					responseData.runningTime = responseData.endTime - responseData.startTime;
					responseData.size = responseData.bodySize;
				}
			};
			
			page.clearMemoryCache();
			loadTime = Date.now();
	
			page.open(address, function (status) {
				if (status !== 'success') {
					system.stderr.writeLine('RES Failed to load the address');
					done();
				} else {
					loadTime = Date.now() - loadTime;
					diagnosticData.loadTime = loadTime;
					done(diagnosticData);
				}
	
				if (thumbnailFile !== '') { 
					page.render(thumbnailFile, { format: 'png' });
				}
			});
		}`, url, thumbnailFile)
}

// GetMeasurements can be used to get measurements data for page.
// It will try to find an available phantomjs thread from pool for nrOfTries.
// If no free thread is found for nrOfTries it will return error.
func (loadTimer *workersPool) GetMeasurements(rawurl string, nrOfTries int, thumbnailsDir string) (*PageMeasurements, error) {
	_, err := url.ParseRequestURI(rawurl)
	if err != nil {
		return nil, err
	}

	if nrOfTries < 1 {
		return nil, fmt.Errorf("You have to specify at least one try to get any result")
	}

	phantom, err := try(nrOfTries, (*loadTimer).getPhantom)
	if err != nil {
		return nil, err
	}

	defer (*loadTimer).releasePhantom(phantom)

	performance, err := getMeasurementsInternal(phantom, rawurl, thumbnailsDir)
	if err != nil {
		return nil, err
	}

	return performance, nil
}

type getFreePhantomFunc func() (*phantomjs.Phantom, error)

func try(maxTries int, fn getFreePhantomFunc) (*phantomjs.Phantom, error) {
	attempt := 1
	for attempt <= maxTries {
		res, err := fn()
		if err == nil {
			return res, nil
		}

		time.Sleep(200 * time.Millisecond)
		attempt++
	}

	return nil, errors.New("Maximum phantom wait time exceeded")
}

func getMeasurementsInternal(phantom *phantomjs.Phantom, rawurl string, thumbnailsDir string) (*PageMeasurements, error) {
	thumbnailFile, err := getThumbnailFile(thumbnailsDir)
	if err != nil {
		return nil, err
	}

	var result interface{}
	err = phantom.Run(getJsFunc(rawurl, thumbnailFile), &result)
	if err != nil {
		return nil, err
	}

	jsonResult, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}

	var performance PageMeasurements
	err = json.Unmarshal(jsonResult, &performance)
	if err != nil {
		return nil, err
	}

	performance.LoadTimeDuration = time.Duration(performance.LoadTime) * time.Millisecond
	if len(thumbnailFile) > 0 {
		performance.ThumbnailFile = thumbnailFile
	}

	for _, v := range performance.Responses {
		v.RunningTimeDuration = time.Duration(v.RunningTime) * time.Millisecond
	}

	return &performance, nil
}

func getThumbnailFile(thumbnailsDir string) (string, error) {
	var thumbnailFile string
	if len(thumbnailsDir) > 0 {
		thumbnail, err := ioutil.TempFile(thumbnailsDir, "thumb")
		if err != nil {
			return "", err
		}
		thumbnailFile = thumbnail.Name()
		thumbnail.Close()
	}
	return thumbnailFile, nil
}
