package scheduler

import (
	"fmt"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/dustin/go-humanize"
	"gitlab.4medica.net/gke/kube-backup/backup"
	"gitlab.4medica.net/gke/kube-backup/config"
	"gitlab.4medica.net/gke/kube-backup/db"
	"gitlab.4medica.net/gke/kube-backup/metrics"
	"gitlab.4medica.net/gke/kube-backup/notifier"
	"github.com/pkg/errors"
	"github.com/robfig/cron"
)

type Scheduler struct {
	Cron    *cron.Cron
	Plans   []config.Plan
	Config  *config.AppConfig
	Stats   *db.StatusStore
	metrics *metrics.BackupMetrics
}

func New(plans []config.Plan, conf *config.AppConfig, stats *db.StatusStore) *Scheduler {
	s := &Scheduler{
		Cron:    cron.New(),
		Plans:   plans,
		Config:  conf,
		Stats:   stats,
		metrics: metrics.New("kubedbbackup", "scheduler"),
	}

	return s
}

func (s *Scheduler) Start() error {
	for _, plan := range s.Plans {
		schedule, err := cron.ParseStandard(plan.Scheduler.Cron)
		if err != nil {
			return errors.Wrapf(err, "Invalid cron %v for plan %v", plan.Scheduler.Cron, plan.Name)
		}
		s.Cron.Schedule(schedule, backupJob{plan.Name, plan, s.Config, s.Stats, s.metrics, s.Cron})
	}

	s.Cron.AddFunc("0 0 */1 * *", func() {
		backup.TmpCleanup(s.Config.TmpPath)
	})
	s.Cron.Start()

	stats := make([]*db.Status, 0)

	for _, e := range s.Cron.Entries() {
		switch e.Job.(type) {
		case backupJob:
			status := &db.Status{
				Plan:    e.Job.(backupJob).name,
				NextRun: e.Next,
			}
			stats = append(stats, status)
		default:
			logrus.Infof("Next tmp cleanup run at %v", e.Next)
		}
	}

	if err := s.Stats.Sync(stats); err != nil {
		logrus.Errorf("Status store sync failed %v", err)
	}

	return nil
}

type backupJob struct {
	name    string
	plan    config.Plan
	conf    *config.AppConfig
	stats   *db.StatusStore
	metrics *metrics.BackupMetrics
	cron    *cron.Cron
}

func (b backupJob) Run() {
	logrus.WithField("plan", b.plan.Name).Info("Job started")
	status := "200"
	log := ""
	t1 := time.Now()

	res, err := backup.Run(b.plan, b.conf.TmpPath, b.conf.StoragePath)
	if err != nil {
		status = "500"
		log = fmt.Sprintf("Job failed %v", err)
		logrus.WithField("plan", b.plan.Name).Error(log)

		if err := notifier.SendNotification(fmt.Sprintf("%v job failed", b.plan.Name),
			err.Error(), true, b.plan); err != nil {
			logrus.WithField("plan", b.plan.Name).Errorf("Notifier failed %v", err)
		}
	} else {
		if res.HandlesBackupData {
			log = fmt.Sprintf("Job finished in %v archive %v size %v",
				res.Duration, res.Name, humanize.Bytes(uint64(res.Size)))
		} else {
			log = fmt.Sprintf("Job finished in %v",
				res.Duration)
		}

		logrus.WithField("plan", b.plan.Name).Info(log)
		if err := notifier.SendNotification(fmt.Sprintf("%v job finished", b.plan.Name),
			log,
			false, b.plan); err != nil {
			logrus.WithField("plan", b.plan.Name).Errorf("Notifier failed %v", err)
		}
	}

	t2 := time.Now()
	b.metrics.Total.WithLabelValues(b.plan.Name, status).Inc()
	b.metrics.Latency.WithLabelValues(b.plan.Name, status).Observe(t2.Sub(t1).Seconds())

	s := &db.Status{
		LastRun:       &res.Timestamp,
		LastRunStatus: status,
		Plan:          b.plan.Name,
		LastRunLog:    log,
	}

	for _, e := range b.cron.Entries() {
		switch e.Job.(type) {
		case backupJob:
			if e.Job.(backupJob).name == b.plan.Name {
				s.NextRun = e.Next
				break
			}
		}
	}

	logrus.WithField("plan", b.plan.Name).Infof("Next run at %v", s.NextRun)
	if err := b.stats.Put(s); err != nil {
		logrus.WithField("plan", b.plan.Name).Errorf("Status store failed %v", err)
	}
}
