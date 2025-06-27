{{/*
Expand the name of the chart.
*/}}
{{- define "torchrun-controller.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "torchrun-controller.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "torchrun-controller.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "torchrun-controller.labels" -}}
helm.sh/chart: {{ include "torchrun-controller.chart" . }}
{{ include "torchrun-controller.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "torchrun-controller.selectorLabels" -}}
app.kubernetes.io/name: {{ include "torchrun-controller.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
control-plane: controller-manager
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "torchrun-controller.serviceAccountName" -}}
{{- if .Values.controller.serviceAccountName }}
{{- .Values.controller.serviceAccountName }}
{{- else }}
{{- include "torchrun-controller.fullname" . }}
{{- end }}
{{- end }}

{{/*
Create the namespace name
*/}}
{{- define "torchrun-controller.namespace" -}}
{{- default .Values.namespace.name .Release.Namespace }}
{{- end }} 