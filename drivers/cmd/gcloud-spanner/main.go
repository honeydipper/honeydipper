package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	spannerAdmin "cloud.google.com/go/spanner/admin/database/apiv1"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	"google.golang.org/api/option"
	spannerAdminSchema "google.golang.org/genproto/googleapis/spanner/admin/database/v1"
)

var (
	// ErrMissingProject means missing project
	ErrMissingProject = errors.New("project required")
	// ErrMissingLocation means missing location
	ErrMissingLocation = errors.New("location required")
	// ErrMissingDB means missing db
	ErrMissingDB = errors.New("db required")
	// ErrMissingBackupOpID means missing backupOpID
	ErrMissingBackupOpID = errors.New("backupOpID required")
	// ErrCreateClient means failure when creating API client
	ErrCreateClient = errors.New("fail to create client")
	// ErrBackupOpNotFound means the backup operation not found
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
var lro *sync.Map

func main() {
	initFlags()
	flag.Parse()

	lro = &sync.Map{}

	driver = dipper.NewDriver(os.Args[1], "gcloud-spanner")
	driver.Commands["backup"] = backup
	driver.Commands["waitForBackup"] = waitForBackup
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
	expireDuration := time.Hour * 24 * 180
	expireStr, ok := dipper.GetMapDataStr(params, "expires")
	if ok && len(expireStr) > 0 {
		var err error
		expireDuration, err = time.ParseDuration(expireStr)
		if err != nil {
			panic(err)
		}
	}
	timeout := time.Duration(1800)
	timeoutStr, ok := m.Labels["timeout"]
	if ok {
		timeoutInt, _ := strconv.Atoi(timeoutStr)
		timeout = time.Duration(timeoutInt)
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
	ctx, cancel := context.WithTimeout(context.Background(), driver.APITimeout*time.Second)
	defer cancel()
	op, err := client.CreateBackup(ctx, req)
	if err != nil {
		dipper.Logger.Panicf("[%s] unable to start the backup %s/%s/%s: %+v", driver.Service, project, instance, db, err)
	}

	backupOpID := strings.Join([]string{"backup", project, instance, db, req.BackupId}, "_")
	waitCtx, cancelWait := context.WithTimeout(context.Background(), timeout*time.Second)
	lro.Store(backupOpID, []interface{}{op, waitCtx})

	go func() {
		defer cancelWait()
		defer lro.Delete(backupOpID)

		_, _ = op.Wait(waitCtx)
	}()

	m.Reply <- dipper.Message{
		Payload: map[string]interface{}{
			"backupOpID": backupOpID,
		},
	}
}

func waitForBackup(m *dipper.Message) {
	m = dipper.DeserializePayload(m)
	params := m.Payload
	backupOpID, ok := dipper.GetMapDataStr(params, "backupOpID")
	if !ok {
		panic(ErrMissingBackupOpID)
	}

	obj, ok := lro.Load(backupOpID)
	if ok {
		op := obj.([]interface{})[0].(*spannerAdmin.CreateBackupOperation)
		waitCtx := obj.([]interface{})[1].(context.Context)
		backup, err := op.Wait(waitCtx)

		if err != nil {
			panic(err)
		}

		m.Reply <- dipper.Message{
			Payload: map[string]interface{}{
				"backup": backup,
			},
		}
	} else {
		panic(ErrBackupOpNotFound)
	}
}
