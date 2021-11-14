package runner

import (
	"fmt"
)

type AppState string

const (
	AppStateReady    AppState = "ready"
	AppStateFailed   AppState = "failed"
	AppStateStarting AppState = "starting"
	AppStateStopping AppState = "stopping"
)

// +k8s:deepcopy-gen=true
type App struct {
	Name   string
	Type   string
	Dir    string
	Host   string
	Status AppStatus
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
