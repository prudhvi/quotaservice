/*
 *   Copyright 2016 Manik Surtani
 *
 *   Licensed under the Apache License, Version 2.0 (the "License");
 *   you may not use this file except in compliance with the License.
 *   You may obtain a copy of the License at
 *
 *       http://www.apache.org/licenses/LICENSE-2.0
 *
 *   Unless required by applicable law or agreed to in writing, software
 *   distributed under the License is distributed on an "AS IS" BASIS,
 *   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *   See the License for the specific language governing permissions and
 *   limitations under the License.
 */

package quotaservice

import (
	"fmt"
	"github.com/maniksurtani/quotaservice/configs"
	"github.com/maniksurtani/quotaservice/buckets"
	"github.com/maniksurtani/quotaservice/lifecycle"
	"github.com/maniksurtani/quotaservice/logging"
	"github.com/maniksurtani/quotaservice/admin"
	"github.com/maniksurtani/quotaservice/clustering"
	"time"
)

type Server struct {
	cfgs            *configs.ServiceConfig
	currentStatus   lifecycle.Status
	stopper         *chan int
	adminServer     *admin.AdminServer
	bucketContainer *buckets.BucketContainer
	bucketFactory   buckets.BucketFactory
	rpcEndpoints    []RpcEndpoint
	monitoring      *Monitoring
}

// NewFromFile creates a new quotaservice server.
func New(config *configs.ServiceConfig, bucketFactory buckets.BucketFactory, rpcEndpoints ...RpcEndpoint) *Server {
	if len(rpcEndpoints) == 0 {
		panic("Need at least 1 RPC endpoint to run the quota service.")
	}
	s := &Server{
		cfgs: config,
		adminServer: admin.NewAdminServer(config.AdminPort),
		bucketFactory: bucketFactory,
		rpcEndpoints: rpcEndpoints}

	// TODO(manik): Metrics? Monitoring? Naming...
	if config.MetricsEnabled {
		s.monitoring = newMonitoring()
	}
	return s
}

func (this *Server) String() string {
	return fmt.Sprintf("Quota Server running with status %v", this.currentStatus)
}

func (this *Server) Start() (bool, error) {

	// Initialize buckets
	this.bucketFactory.Init(this.cfgs)
	this.bucketContainer = buckets.NewBucketContainer(this.cfgs, this.bucketFactory)
	// Start the admin server
	this.adminServer.Start()

	// Start the RPC servers
	for _, rpcServer := range this.rpcEndpoints {
		rpcServer.Init(this)
		rpcServer.Start()
	}

	this.currentStatus = lifecycle.Started
	return true, nil
}

func (this *Server) Stop() (bool, error) {
	this.currentStatus = lifecycle.Stopped

	// Stop the admin server
	this.adminServer.Stop()

	// Stop the RPC servers
	for _, rpcServer := range this.rpcEndpoints {
		rpcServer.Stop()
	}

	return true, nil
}

func (this *Server) Allow(namespace string, name string, tokensRequested int) (granted int, waitTime int64, err error) {
	b := this.bucketContainer.FindBucket(namespace, name)
	// TODO(manik) Fix contracts, searching for buckets, etc.
	if b == nil {
		err = newError(fmt.Sprintf("No such bucket %v:%v in namespace %v", namespace, name), ER_NO_SUCH_BUCKET)
		return
	}

	dur := time.Millisecond * time.Duration(b.GetConfig().WaitTimeoutMillis)
	waitTimeNanos := b.Take(tokensRequested, dur).Nanoseconds()
	waitTime = (waitTimeNanos % 1e9) / 1e6

	if waitTime < 0 {
		err = newError(fmt.Sprintf("Timed out waiting on %v:%v in ", namespace, name), ER_TIMED_OUT_WAITING)
	} else {
		granted = tokensRequested
	}

	return
}

func (this *Server) GetMonitoring() *Monitoring {
	return this.monitoring
}

func (this *Server) SetLogger(logger logging.Logger) {
	logging.SetLogger(logger)
}

func (this *Server) SetClustering(clustering clustering.Clustering) {
	// TODO(manik): Implement me
}

