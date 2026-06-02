{{/*
Expand the name of the chart.
*/}}
{{- define "ack-event-publisher.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "ack-event-publisher.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- .Chart.Name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{/*
Common labels applied to every resource created by this chart.
*/}}
{{- define "ack-event-publisher.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
app.kubernetes.io/name: {{ include "ack-event-publisher.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
k8s-app: {{ include "ack-event-publisher.name" . }}
{{- end }}

{{/*
Selector labels used by the Deployment and Service.
*/}}
{{- define "ack-event-publisher.selectorLabels" -}}
app.kubernetes.io/name: {{ include "ack-event-publisher.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
ServiceAccount name.
*/}}
{{- define "ack-event-publisher.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "ack-event-publisher.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Container image including tag. Falls back to Chart.AppVersion.
*/}}
{{- define "ack-event-publisher.image" -}}
{{ .Values.image.repository }}:{{ default .Chart.AppVersion .Values.image.tag }}
{{- end }}
