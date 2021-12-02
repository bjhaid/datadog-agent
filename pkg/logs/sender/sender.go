// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sender

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

type destinationState struct {
	isRetrying        bool
	input             chan *message.Payload
	retryStateChanged chan bool
}

func (d *destinationState) updateAndGetIsRetrying() bool {
	select {
	case d.isRetrying = <-d.retryStateChanged:
	default:
	}
	return d.isRetrying
}

func (d *destinationState) close() {
	close(d.input)
	close(d.retryStateChanged)
}

// Sender sends logs to different destinations.
type Sender struct {
	inputChan    chan *message.Payload
	outputChan   chan *message.Payload
	destinations *client.Destinations
	done         chan struct{}
	stop         chan struct{}
	bufferSize   int
}

// NewSender returns a new sender.
func NewSender(inputChan chan *message.Payload, outputChan chan *message.Payload, destinations *client.Destinations, bufferSize int) *Sender {
	return &Sender{
		inputChan:    inputChan,
		outputChan:   outputChan,
		destinations: destinations,
		done:         make(chan struct{}),
		stop:         make(chan struct{}, 1),
		bufferSize:   bufferSize,
	}
}

// Start starts the sender.
func (s *Sender) Start() {
	go s.run()
}

// Stop stops the sender,
// this call blocks until inputChan is flushed
func (s *Sender) Stop() {
	close(s.inputChan)
	s.stop <- struct{}{}
	<-s.done
}

func (s *Sender) run() {
	reliableDestinations := buildDestinationStates(s.destinations.Reliable, s.outputChan, s.bufferSize)

	sink := additionalDestinationsSink(s.bufferSize)
	additionalDestinations := buildDestinationStates(s.destinations.Additionals, sink, s.bufferSize)

payloadLoop:
	for payload := range s.inputChan {
		select {
		case <-s.stop:
			break payloadLoop
		default:
		}

		sent := false
		for !sent {
			select {
			case <-s.stop:
				break payloadLoop
			default:
				for _, destState := range reliableDestinations {
					if !destState.updateAndGetIsRetrying() {
						destState.input <- payload
						sent = true
					}
				}

				if !sent {
					// Using a busy loop is much simpler than trying to join an arbitrary number of channels and
					// wait for just one of them. This is an exceptional case so it has little overhead since it
					// will only happen when there is no possible way to send logs.
					time.Sleep(100 * time.Millisecond)
				}
			}
		}

		for _, destState := range reliableDestinations {
			// if an endpoint is stuck in the previous step, try to buffer the payloads if we have room to mitigate
			// loss on intermittent failures.
			if destState.isRetrying {
				select {
				case destState.input <- payload:
				default:
				}
			}
		}

		// Attempt to send to additional destination
		for _, destState := range additionalDestinations {
			select {
			case destState.input <- payload:
			default:
			}
		}
	}

	// Cleanup
	for _, destState := range reliableDestinations {
		destState.close()
	}
	for _, destState := range additionalDestinations {
		destState.close()
	}
	s.done <- struct{}{}
}

// Drains the output channel from destinations that don't update the auditor.
func additionalDestinationsSink(bufferSize int) chan *message.Payload {
	sink := make(chan *message.Payload, bufferSize)
	go func() {
		for {
			<-sink
		}
	}()
	return sink
}

func buildDestinationStates(destinations []client.Destination, output chan *message.Payload, bufferSize int) []*destinationState {
	states := []*destinationState{}
	for _, input := range destinations {
		destState := &destinationState{false, make(chan *message.Payload, bufferSize), make(chan bool, 1)}
		states = append(states, destState)
		input.Start(destState.input, destState.retryStateChanged, output)
	}
	return states
}
