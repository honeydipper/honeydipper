package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	spannerAdmin "cloud.google.com/go/spanner/admin/database/apiv1"
	spannerAdminSchema "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/honeydipper/honeydipper/v4/pkg/dipper"
	"google.golang.org/api/option"
)

const (
	// DefaultBackupOpWaitTimeout is the default timeout in seconds for waiting for the backup to complete.
	DefaultBackupOpWaitTimeout = "30m"

	// DefaultBackupExpireDuration is the default duration before a backup is expired and removed.
	DefaultBackupExpire = "4320h" // 24 * 180 = 4320
)

var (
	// ErrMissingProject means missing project.
	ErrMissingProject = errors.New("project required")
	// ErrMissingLocation means missing location.
	ErrMissingLocation = errors.New("location required")
	// ErrMissingDB means missing db.
	ErrMissingDB = errors.New("db required")
	// ErrMissingBackupOpID means missing backupOpID.
	ErrMissingBackupOpID = errors.New("backupOpID required")
	// ErrCreateClient means failure when creating API client.
	ErrCreateClient = errors.New("fail to create client")
	// ErrBackupOpNotFound means the backup operation not found.
	ErrBackupOpNotFound = errors.New("backup op not found")
)

func initFlags() {
	flag.Usage = func() {
		fmt.Printf("%s [ -h ] <service name>\n", os.Args[0])
		fmt.Printf("    This driver supports operator service.\n")
		fmt.Printf("  This program provides honeydipper with capability of interacting with gcloud spanner\n")
	}
}

var driver *dipper.Driver

func main() {
	initFlags()
	flag.Parse()

	driver = dipper.NewDriver(os.Args[1], "gcloud-spanner")
	driver.Commands["backup"] = backup
	driver.Commands["waitForBackup|interruptible"] = waitForBackup
	driver.DefaultTimeout["waitForBackup"] = DefaultBackupOpWaitTimeout
	driver.Run()
}

func backup(m *dipper.Message) {
	m = dipper.DeserializePayload(m)
	params := m.Payload
	serviceAccountBytes, _ := dipper.GetMapDataStr(params, "service_account")
	project, ok := dipper.GetMapDataStr(params, "project")
	if !ok {
		panic(ErrMissingProject)
	}
	instance, ok := dipper.GetMapDataStr(params, "instance")
	if !ok {
		panic(ErrMissingLocation)
	}
	db, ok := dipper.GetMapDataStr(params, "db")
	if !ok {
		panic(ErrMissingDB)
	}
	expire, ok := dipper.GetMapDataStr(params, "expires")
	if !ok {
		expire = DefaultBackupExpire
	}
	expireDuration := dipper.Must(time.ParseDuration(expire)).(time.Duration)

	var (
		client *spannerAdmin.DatabaseAdminClient
		err    error
	)
	if len(serviceAccountBytes) > 0 {
		clientOption := option.WithCredentialsJSON([]byte(serviceAccountBytes))
		client, err = spannerAdmin.NewDatabaseAdminClient(context.Background(), clientOption)
	} else {
		client, err = spannerAdmin.NewDatabaseAdminClient(context.Background())
	}
	if err != nil {
		panic(ErrCreateClient)
	}

	t := time.Now().Add(expireDuration)
	expireTime := &timestamp.Timestamp{Seconds: t.Unix(), Nanos: int32(t.Nanosecond())}
	req := &spannerAdminSchema.CreateBackupRequest{
		Parent:   fmt.Sprintf("projects/%s/instances/%s", project, instance),
		BackupId: time.Now().Format("b20060102030405"),
		Backup: &spannerAdminSchema.Backup{
			Database:   fmt.Sprintf("projects/%s/instances/%s/databases/%s", project, instance, db),
			ExpireTime: expireTime,
		},
	}

	ctx, cancel := driver.GetContext(m)
	defer cancel()
	op, err := client.CreateBackup(ctx, req)
	if err != nil {
		dipper.Logger.Panicf("[%s] unable to start the backup %s/%s/%s: %+v", driver.Service, project, instance, db, err)
	}

	backupOpID := op.Name()

	m.Reply <- dipper.Message{
		Payload: map[string]interface{}{
			"backupOpID": backupOpID,
		},
	}
}

func waitForBackup(m *dipper.Message) {
	m = dipper.DeserializePayload(m)
	params := m.Payload
	serviceAccountBytes, _ := dipper.GetMapDataStr(params, "service_account")

	backupOpID, ok := dipper.GetMapDataStr(params, "backupOpID")
	if !ok {
		panic(ErrMissingBackupOpID)
	}

	var (
		client *spannerAdmin.DatabaseAdminClient
		err    error
	)
	if len(serviceAccountBytes) > 0 {
		clientOption := option.WithCredentialsJSON([]byte(serviceAccountBytes))
		client, err = spannerAdmin.NewDatabaseAdminClient(context.Background(), clientOption)
	} else {
		client, err = spannerAdmin.NewDatabaseAdminClient(context.Background())
	}

	if err != nil {
		panic(ErrCreateClient)
	}

	op := client.CreateBackupOperation(backupOpID)

	ctx, cancel := driver.GetContext(m)
	defer cancel()

	backup := dipper.Must(op.Wait(ctx)).(*spannerAdminSchema.Backup)

	m.Reply <- dipper.Message{
		Payload: map[string]interface{}{
			"backup": backup,
		},
	}
}
