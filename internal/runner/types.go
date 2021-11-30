package runner

import (
	"fmt"
)

type AppState int

const (
	AppStateUnknown  AppState = iota
	AppStateDisabled AppState = iota
	AppStateExited   AppState = iota
	AppStateReady    AppState = iota
	AppStateStarting AppState = iota
	AppStateStopping AppState = iota
	AppStateError    AppState = iota
)

var (
	states = []string{
		"unknown",
		"disabled",
		"exited",
		"ready",
		"starting",
		"stopping",
		"failed",
	}
)

func (a AppState) String() string {
	return states[int(a)]
}

// +k8s:deepcopy-gen=true
type App struct {
	Name    string
	Type    string
	Host    string
	Enabled bool
	Status  AppStatus
}

func (a *App) DeepCopyIntoResource(dest Resource) error {
	out, ok := dest.(*App)
	if !ok {
		return fmt.Errorf("deep copy: unexpected target type %T provided, expected %T", dest, a)
	}
	a.DeepCopyInto(out)
	return nil
}

// +k8s:deepcopy-gen=true
type AppList struct {
	Items []App
}

func (a *AppList) SetItems(items []Resource) error {
	a.Items = make([]App, len(items))
	for i, item := range items {
		if err := item.DeepCopyIntoResource(&a.Items[i]); err != nil {
			return err
		}
	}
	return nil
}

// +k8s:deepcopy-gen=true
type AppStatus struct {
	State      AppState
	Containers []AppContainer
}

// +k8s:deepcopy-gen=true
type AppContainer struct {
	ID    string
	Name  string
	State AppState
}
