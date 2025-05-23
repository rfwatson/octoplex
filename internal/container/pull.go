package container

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"

	"git.netflux.io/rob/octoplex/internal/domain"
	"github.com/docker/docker/api/types/image"
)

func handleImagePull(
	ctx context.Context,
	imageName string,
	dockerClient DockerClient,
	containerStateC chan<- domain.Container,
	logger *slog.Logger,
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

	var (
		pp     pullProgress
		logged bool
	)

	for {
		if err := pullDecoder.Decode(&pp); err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("image pull: %w", err)
		}

		if pp.Progress != "" {
			if !logged {
				// only log if the image is actually being pulled
				logger.Info("Pulling image", "image", imageName)
				defer logger.Info("Pulling image complete", "image", imageName)

				logged = true
			}

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
