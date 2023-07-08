package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type Device struct {
	IP       string
	User     string
	IsOnline bool // New field to track device status
}

var (
	targetDevices = []Device{
		{IP: "192.168.1.101", User: "chris"}, // Replace with the IP and username for your devices
		// {IP: "192.168.1.101", User: "james"}, // Add more devices as needed
		// {IP: "192.168.1.102", User: "sarah"},
	}
	scanPeriod = 10 * time.Second // Adjust the interval between scans as needed
)

const (
	StateDisconnected = "Disconnected"
	StateConnected    = "Connected"
)

type FSM struct {
	state     map[string]string // Map to track device states
	mutex     sync.Mutex        // Mutex to synchronize access to state and device fields
	transFunc map[string]func(device *Device) string
}

func NewFSM() *FSM {
	fsm := &FSM{
		state: make(map[string]string),
		transFunc: map[string]func(device *Device) string{
			StateDisconnected: disconnectedTransition,
			StateConnected:    connectedTransition,
		},
	}
	return fsm
}

func (fsm *FSM) transition(device *Device) {
	currentState := fsm.getState(device.IP)
	newState := fsm.transFunc[currentState](device)
	fsm.setState(device.IP, newState)
}

func disconnectedTransition(device *Device) string {
	// Perform a ping check to verify device availability
	if err := pingCheck(device.IP); err == nil {
		if !device.IsOnline {
			// Device is up and was previously down
			fmt.Printf("Device (%s) is up\n", device.IP)
			launchGnomeTerminal(device)
		}
		device.IsOnline = true
		return StateConnected
	}
	device.IsOnline = false
	return StateDisconnected
}

func connectedTransition(device *Device) string {
	// Perform a ping check to verify device availability
	if err := pingCheck(device.IP); err == nil {
		// Device is still connected
		fmt.Printf("Device (%s) is still connected\n", device.IP)

		if !device.IsOnline {
			// The gnome-terminal window was closed
			fmt.Printf("Device (%s) gnome-terminal closed\n", device.IP)
			go func() {
				for {
					if device.IsOnline {
						fmt.Printf("Device (%s) is still connected\n", device.IP)
						launchGnomeTerminal(device)
					} else {
						break
					}
				}
			}()
		}
		device.IsOnline = true
	} else {
		// Device is disconnected
		fmt.Printf("Device (%s) is disconnected\n", device.IP)
		device.IsOnline = false
		return StateDisconnected
	}
	return StateConnected
}

func main() {
	// Create a context with cancellation capability
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a new FSM
	fsm := NewFSM()

	// Start the monitoring goroutine
	go monitor(ctx, fsm)

	// Wait for termination signal
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGINT, syscall.SIGTERM)
	<-signalCh

	// Perform cleanup operations
	fmt.Println("Shutting down...")
	// Add any necessary cleanup logic here
}

// Monitor function runs in a separate goroutine and performs the monitoring
func monitor(ctx context.Context, fsm *FSM) {
	ticker := time.NewTicker(scanPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Context is canceled, terminate the goroutine
			return
		case <-ticker.C:
			// Continue monitoring

			for i := range targetDevices {
				device := &targetDevices[i] // Take the address of the device for updating IsOnline
				fsm.transition(device)
			}
		}
	}
}

// Perform a ping check to verify device availability
func pingCheck(ip string) error {
	cmd := exec.Command("ping", "-c", "1", "-W", "1", ip) // Adjust the ping command for your operating system
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("ping check failed: %w", err)
	}
	return nil
}

// Launch gnome-terminal and connect via SSH
func launchGnomeTerminal(device *Device) {
	sshCommand := fmt.Sprintf("ssh %s@%s", device.User, device.IP)
	cmd := exec.Command("gnome-terminal", "-e", sshCommand)
	if err := cmd.Start(); err != nil {
		fmt.Println("Failed to launch terminal:", err)
	}

	go func() {
		// Wait for the gnome-terminal process to finish
		err := cmd.Wait()
		if err != nil {
			fmt.Printf("Device (%s) gnome-terminal closed\n", device.IP)

			// Set device offline
			device.IsOnline = false
		}
	}()
}

// Helper function to get the state for a given device IP
func (fsm *FSM) getState(ip string) string {
	fsm.mutex.Lock()
	defer fsm.mutex.Unlock()

	state, ok := fsm.state[ip]
	if !ok {
		// Default state is Disconnected
		state = StateDisconnected
	}
	return state
}

// Helper function to set the state for a given device IP
func (fsm *FSM) setState(ip string, newState string) {
	fsm.mutex.Lock()
	defer fsm.mutex.Unlock()

	fsm.state[ip] = newState
}
