package wails

import (
	"context"
	"fmt"

	"github.com/nord-launcher/launcher/internal/core/domain"
	"github.com/nord-launcher/launcher/internal/core/launch"
)

// WailsAdapter connects Wails IPC layer to the Hexagonal Core.
type WailsAdapter struct {
	svc *launch.InstanceService
}

func NewWailsAdapter(svc *launch.InstanceService) *WailsAdapter {
	return &WailsAdapter{svc: svc}
}

func (a *WailsAdapter) ListInstances() []InstanceDTO {
	coreList := a.svc.ListInstances()
	dtoList := make([]InstanceDTO, 0, len(coreList))
	for _, inst := range coreList {
		dtoList = append(dtoList, InstanceDTO{
			ID:               inst.ID,
			Name:             inst.Name,
			GameVersion:      inst.GameVersion,
			Loader:           string(inst.Loader),
			LoaderVersion:    inst.LoaderVer,
			IconPath:         inst.IconPath,
			State:            string(inst.State),
			TotalPlaySeconds: inst.TotalPlaySec,
		})
	}
	return dtoList
}

func (a *WailsAdapter) CreateInstance(req CreateInstanceRequest) (*InstanceDTO, error) {
	inst, err := a.svc.CreateInstance(req.Name, req.GameVersion, domain.LoaderType(req.Loader))
	if err != nil {
		return nil, fmt.Errorf("create instance: %w", err)
	}

	return &InstanceDTO{
		ID:               inst.ID,
		Name:             inst.Name,
		GameVersion:      inst.GameVersion,
		Loader:           string(inst.Loader),
		LoaderVersion:    inst.LoaderVer,
		IconPath:         inst.IconPath,
		State:            string(inst.State),
		TotalPlaySeconds: inst.TotalPlaySec,
	}, nil
}

func (a *WailsAdapter) LaunchInstance(id string) (*LaunchResponse, error) {
	pid, err := a.svc.Launch(context.Background(), id)
	if err != nil {
		return &LaunchResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return &LaunchResponse{
		Success: true,
		PID:     pid,
	}, nil
}
