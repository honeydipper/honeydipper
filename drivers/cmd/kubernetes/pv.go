// Copyright 2023 PayPal Inc.

// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT License was not distributed with this file,
// you can obtain one at https://mit-license.org/.

// Package honeydipper is an event-driven, rule based orchestration platform tailor towards
// DevOps and system engineering workflows.
package main

import (
	"github.com/ghodss/yaml"
	"github.com/honeydipper/honeydipper/pkg/dipper"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func createPVC(m *dipper.Message) {
	k8sclient := prepareKubeConfig(m)
	nameSpace, ok := dipper.GetMapDataStr(m.Payload, "namespace")
	if !ok {
		nameSpace = DefaultNamespace
	}

	pvc := corev1.PersistentVolumeClaim{}
	source := dipper.MustGetMapData(m.Payload, "pvc")
	buf, err := yaml.Marshal(source)
	if err != nil {
		log.Panicf("[%s] unable to marshal pvc manifest %+v", driver.Service, err)
	}
	err = yaml.Unmarshal(buf, &pvc)
	if err != nil {
		log.Panicf("[%s] invalid pvc manifest %+v", driver.Service, err)
	}

	client := k8sclient.CoreV1().PersistentVolumeClaims(nameSpace)
	ctx, cancel := driver.GetContext()
	defer cancel()
	pvcResult, e := client.Create(ctx, &pvc, metav1.CreateOptions{})
	if e != nil {
		log.Panicf("[%s] failed to create pvc %+v", driver.Service, e)
	}

	m.Reply <- dipper.Message{
		Payload: map[string]interface{}{
			"metadata": pvcResult.ObjectMeta,
			"status":   pvcResult.Status,
		},
	}
}

func deletePVC(m *dipper.Message) {
	k8sclient := prepareKubeConfig(m)
	nameSpace, ok := dipper.GetMapDataStr(m.Payload, "namespace")
	if !ok {
		nameSpace = DefaultNamespace
	}
	pvcName := dipper.MustGetMapDataStr(m.Payload, "pvc")

	client := k8sclient.CoreV1().PersistentVolumeClaims(nameSpace)
	ctx, cancel := driver.GetContext()
	defer cancel()
	e := client.Delete(ctx, pvcName, metav1.DeleteOptions{})
	if e != nil {
		log.Panicf("[%s] failed to delete pvc %+v", driver.Service, e)
	}

	m.Reply <- dipper.Message{}
}
