{{/*
Expand the name of the chart.
*/}}
{{- define "taas.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "taas.fullname" -}}
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
Create chart label.
*/}}
{{- define "taas.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels.
*/}}
{{- define "taas.labels" -}}
helm.sh/chart: {{ include "taas.chart" . }}
{{ include "taas.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels.
*/}}
{{- define "taas.selectorLabels" -}}
app.kubernetes.io/name: {{ include "taas.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Service-specific labels (pass service name as .service).
Usage: {{ include "taas.serviceLabels" (dict "service" "gateway" "root" .) }}
*/}}
{{- define "taas.serviceLabels" -}}
helm.sh/chart: {{ include "taas.chart" .root }}
app.kubernetes.io/name: {{ printf "%s-%s" (include "taas.name" .root) .service }}
app.kubernetes.io/instance: {{ .root.Release.Name }}
app.kubernetes.io/component: {{ .service }}
app.kubernetes.io/version: {{ .root.Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .root.Release.Service }}
app.kubernetes.io/part-of: taas
{{- end }}

{{/*
Image reference helper.
Usage: {{ include "taas.image" (dict "repo" .Values.gateway.image.repository "root" .) }}
*/}}
{{- define "taas.image" -}}
{{- printf "%s/%s:%s" .root.Values.global.imageRegistry .repo .root.Values.image.tag }}
{{- end }}
