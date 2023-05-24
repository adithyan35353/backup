package backup

import (
	"fmt"
	"github.com/codeskyblue/go-sh"
	"github.com/pkg/errors"
)

func applyRetention(path string, retention, logRetention int) error {
	gz := fmt.Sprintf("cd %v && rm -f $(ls -1t *.gz *.tgz | tail -n +%v)", path, retention+1)
	err := sh.Command("/bin/sh", "-c", gz).Run()

	if err != nil {
		return errors.Wrapf(err, "removing old gz files from %v failed", path)
	}

	log := fmt.Sprintf("cd %v && rm -f $(ls -1t *.log | tail -n +%v)", path, logRetention+1)
	err = sh.Command("/bin/sh", "-c", log).Run()

	if err != nil {
		return errors.Wrapf(err, "removing old log files from %v failed", path)
	}

	return nil
}

// TmpCleanup remove files older than one day
func TmpCleanup(path string) error {
	rm := fmt.Sprintf("find %v -not -name \"kube-backup.db\" -mtime +%v -type f -delete", path, 1)
	err := sh.Command("/bin/sh", "-c", rm).Run()

	if err != nil {
		return errors.Wrapf(err, "%v cleanup failed", path)
	}

	return nil
}
