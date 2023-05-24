package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/dustin/go-humanize"
	"github.com/go-chi/chi"
	"github.com/go-chi/render"
	"gitlab.4medica.net/gke/kube-backup/backup"
	"gitlab.4medica.net/gke/kube-backup/config"
	"gitlab.4medica.net/gke/kube-backup/notifier"
)

func configCtx(data config.AppConfig) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(context.WithValue(r.Context(), "app.config", data))
			next.ServeHTTP(w, r)
		})
	}
}

func postBackup(w http.ResponseWriter, r *http.Request) {
	cfg := r.Context().Value("app.config").(config.AppConfig)
	planID := chi.URLParam(r, "planID")
	plan, err := config.LoadPlan(cfg.ConfigPath, planID)
	if err != nil {
		render.Status(r, 500)
		render.JSON(w, r, map[string]string{"error": err.Error()})
		return
	}

	logrus.WithField("plan", planID).Info("On demand job started")

	res, err := backup.Run(plan, cfg.TmpPath, cfg.StoragePath)
	if err != nil {
		logrus.WithField("plan", planID).Errorf("On demand job failed %v", err)

		if err := notifier.SendNotification(fmt.Sprintf("%v on demand job failed", planID),
			err.Error(), true, plan); err != nil {
			logrus.WithField("plan", plan.Name).Errorf("Notifier failed for on demand job %v", err)
		}

		render.Status(r, 500)
		render.JSON(w, r, map[string]string{"error": err.Error()})
	} else {
		if res.HandlesBackupData {
			logrus.WithField("plan", plan.Name).Infof("On demand job finished in %v archive %v size %v",
				res.Duration, res.Name, humanize.Bytes(uint64(res.Size)))
		} else {
			logrus.WithField("plan", plan.Name).Infof("On demand job finished in %v",
				res.Duration)
		}

		if err := notifier.SendNotification(fmt.Sprintf("%v on demand job finished", plan.Name),
			fmt.Sprintf("%v job finished in %v archive size %v",
				res.Name, res.Duration, humanize.Bytes(uint64(res.Size))),
			false, plan); err != nil {
			logrus.WithField("plan", plan.Name).Errorf("Notifier failed for on demand job %v", err)
		}

		render.JSON(w, r, toBackupResult(res))
	}
}

type backupResult struct {
	Plan      string    `json:"plan"`
	File      string    `json:"file,omitempty"`
	Duration  string    `json:"duration"`
	Size      string    `json:"size,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

func toBackupResult(res backup.Result) backupResult {
	var size string

	if res.HandlesBackupData {
		size = humanize.Bytes(uint64(res.Size))
	}

	return backupResult{
		Plan:      res.Plan,
		Duration:  fmt.Sprintf("%v", res.Duration),
		File:      res.Name,
		Size:      size,
		Timestamp: res.Timestamp,
	}
}
