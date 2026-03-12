// Copyright (c) Codesphere Inc.
// SPDX-License-Identifier: Apache-2.0

package util_test

import (
	"github.com/codesphere-cloud/oms/internal/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var _ = Describe("decodeMultiDocYAML", func() {
	It("decodes a single YAML document", func() {
		yaml := []byte(`
apiVersion: v1
kind: Secret
metadata:
  name: my-secret
  namespace: argocd
`)
		objects, err := util.DecodeMultiDocYAML(yaml)
		Expect(err).ToNot(HaveOccurred())
		Expect(objects).To(HaveLen(1))
		Expect(objects[0].GetName()).To(Equal("my-secret"))
		Expect(objects[0].GetNamespace()).To(Equal("argocd"))
		Expect(objects[0].GetKind()).To(Equal("Secret"))
	})

	It("decodes multiple YAML documents separated by ---", func() {
		yaml := []byte(`
apiVersion: argoproj.io/v1alpha1
kind: AppProject
metadata:
  name: prod
  namespace: argocd
---
apiVersion: argoproj.io/v1alpha1
kind: AppProject
metadata:
  name: dev
  namespace: argocd
---
apiVersion: argoproj.io/v1alpha1
kind: AppProject
metadata:
  name: default
  namespace: argocd
`)
		objects, err := util.DecodeMultiDocYAML(yaml)
		Expect(err).ToNot(HaveOccurred())
		Expect(objects).To(HaveLen(3))
		Expect(objects[0].GetName()).To(Equal("prod"))
		Expect(objects[1].GetName()).To(Equal("dev"))
		Expect(objects[2].GetName()).To(Equal("default"))
	})

	It("skips empty documents", func() {
		yaml := []byte(`
---
apiVersion: v1
kind: Secret
metadata:
  name: only-one
  namespace: argocd
---
`)
		objects, err := util.DecodeMultiDocYAML(yaml)
		Expect(err).ToNot(HaveOccurred())
		Expect(objects).To(HaveLen(1))
		Expect(objects[0].GetName()).To(Equal("only-one"))
	})

	It("returns empty slice for empty input", func() {
		objects, err := util.DecodeMultiDocYAML([]byte(""))
		Expect(err).ToNot(HaveOccurred())
		Expect(objects).To(BeEmpty())
	})

	It("returns an error for invalid YAML", func() {
		yaml := []byte(`not: valid: yaml: [`)
		_, err := util.DecodeMultiDocYAML(yaml)
		Expect(err).To(HaveOccurred())
	})
})

var _ = Describe("renderTemplate", func() {
	It("replaces a single variable", func() {
		tpl := []byte(`name: "dc-${DC_NUMBER}"`)
		rendered, err := util.RenderTemplate(tpl, map[string]string{
			"DC_NUMBER": "5",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(string(rendered)).To(Equal(`name: "dc-5"`))
	})

	It("replaces multiple variables", func() {
		tpl := []byte(`server: ${HOST}, port: ${PORT}`)
		rendered, err := util.RenderTemplate(tpl, map[string]string{
			"HOST": "localhost",
			"PORT": "8080",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(string(rendered)).To(Equal(`server: localhost, port: 8080`))
	})

	It("replaces multiple occurrences of the same variable", func() {
		tpl := []byte(`a: ${VAR}, b: ${VAR}`)
		rendered, err := util.RenderTemplate(tpl, map[string]string{
			"VAR": "value",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(string(rendered)).To(Equal(`a: value, b: value`))
	})

	It("leaves unmatched placeholders intact", func() {
		tpl := []byte(`name: ${KNOWN}, secret: ${UNKNOWN}`)
		rendered, err := util.RenderTemplate(tpl, map[string]string{
			"KNOWN": "replaced",
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(string(rendered)).To(ContainSubstring("replaced"))
		Expect(string(rendered)).To(ContainSubstring("${UNKNOWN}"))
	})

	It("returns input unchanged when vars map is empty", func() {
		tpl := []byte(`nothing: to replace`)
		rendered, err := util.RenderTemplate(tpl, map[string]string{})
		Expect(err).ToNot(HaveOccurred())
		Expect(string(rendered)).To(Equal(`nothing: to replace`))
	})
})

var _ = Describe("gvrForUnstructured", func() {
	It("returns the correct GVR for AppProject", func() {
		obj := &unstructured.Unstructured{}
		obj.SetAPIVersion("argoproj.io/v1alpha1")
		obj.SetKind("AppProject")

		gvr, err := util.GvrForUnstructured(obj)
		Expect(err).ToNot(HaveOccurred())
		Expect(gvr.Group).To(Equal("argoproj.io"))
		Expect(gvr.Version).To(Equal("v1alpha1"))
		Expect(gvr.Resource).To(Equal("appprojects"))
	})

	It("returns an error for an unknown kind", func() {
		obj := &unstructured.Unstructured{}
		obj.SetAPIVersion("v1")
		obj.SetKind("ConfigMap")

		_, err := util.GvrForUnstructured(obj)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("no GVR mapping"))
	})
})
