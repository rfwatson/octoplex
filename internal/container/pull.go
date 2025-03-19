package container

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"git.netflux.io/rob/octoplex/internal/domain"
	"github.com/docker/docker/api/types/image"
)

func handleImagePull(
	ctx context.Context,
	imageName string,
	dockerClient DockerClient,
	containerStateC chan<- domain.Container,
) error {
	containerStateC <- domain.Container{
		Status:     domain.ContainerStatusPulling,
		ImageName:  imageName,
		PullStatus: "Waiting",
	}

	pullReader, err := dockerClient.ImagePull(ctx, imageName, image.PullOptions{})
	if err != nil {
		return err
	}

	pullDecoder := json.NewDecoder(pullReader)
	var pp pullProgress
	for {
		if err := pullDecoder.Decode(&pp); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("image pull: %w", err)
		}

		if pp.Progress != "" {
			containerStateC <- domain.Container{
				Status:       domain.ContainerStatusPulling,
				ImageName:    imageName,
				PullStatus:   pp.Status,
				PullProgress: pp.Progress,
				PullPercent:  int(pp.Detail.Curr * 100 / pp.Detail.Total),
			}
		}
	}
	_ = pullReader.Close()

	return nil
}
