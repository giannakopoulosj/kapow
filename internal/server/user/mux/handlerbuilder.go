/*
 * Copyright 2019 Banco Bilbao Vizcaya Argentaria, S.A.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package mux

import (
	"bufio"
	"net/http"
	"os"

	"github.com/google/uuid"

	"github.com/BBVA/kapow/internal/logger"
	"github.com/BBVA/kapow/internal/server/data"
	"github.com/BBVA/kapow/internal/server/model"
	"github.com/BBVA/kapow/internal/server/user/spawn"
)

var spawner = spawn.Spawn
var idGenerator = uuid.NewUUID

func handlerBuilder(route model.Route) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := idGenerator()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		h := &model.Handler{
			ID:      id.String(),
			Route:   route,
			Request: r,
			Writer:  w,
			Status:  200,
		}

		data.Handlers.Add(h)
		defer data.Handlers.Remove(h.ID)

		if route.Debug {
			var stdOutR, stdOutW *os.File
			stdOutR, stdOutW, err = os.Pipe()
			if err != nil {
				logger.L.Println(err)
				return
			}
			// FIX: Check the error on Close
			defer func() {
				if err := stdOutW.Close(); err != nil {
					logger.L.Printf("failed to close stdout writer for handler %s: %v", h.ID, err)
				}
			}()

			var stdErrR, stdErrW *os.File
			stdErrR, stdErrW, err = os.Pipe()
			if err != nil {
				logger.L.Println(err)
				return
			}
			// FIX: Check the error on Close
			defer func() {
				if err := stdErrW.Close(); err != nil {
					logger.L.Printf("failed to close stderr writer for handler %s: %v", h.ID, err)
				}
			}()

			go logStream(h.ID, "stdout", stdOutR)
			go logStream(h.ID, "stderr", stdErrR)

			err = spawner(h, stdOutW, stdErrW)
		} else {
			err = spawner(h, nil, nil)
		}

		// In case of the user not setting /request/body
		if !h.BodyOut {
			if h.Status != 0 {
				h.Writer.WriteHeader(h.Status)
			}
			h.BodyOut = true
		}

		if err != nil {
			logger.L.Println(err)
		}

		if r != nil {
			logger.LogAccess(
				r.RemoteAddr,
				h.ID,
				"-",
				r.Method,
				r.RequestURI,
				r.Proto,
				h.Status,
				h.SentBytes,
				r.Header.Get("Referer"),
				r.Header.Get("User-Agent"),
			)
		}
	})
}

func logStream(handlerId string, streamName string, stream *os.File) {
	// FIX: Check the error on Close
	defer func() {
		if err := stream.Close(); err != nil {
			logger.L.Printf("failed to close %s stream for handler %s: %v", streamName, handlerId, err)
		}
	}()

	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		logger.L.Printf("%s %s: %s", handlerId, streamName, scanner.Text())
	}
}
