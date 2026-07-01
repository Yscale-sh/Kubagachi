{{/*
Expand the name of the chart.
*/}}
{{- define "kubagachi.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this
(by the DNS naming spec).
*/}}
{{- define "kubagachi.fullname" -}}
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
{{- define "kubagachi.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "kubagachi.labels" -}}
helm.sh/chart: {{ include "kubagachi.chart" . }}
{{ include "kubagachi.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "kubagachi.selectorLabels" -}}
app.kubernetes.io/name: {{ include "kubagachi.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
The name of the ServiceAccount to use.
*/}}
{{- define "kubagachi.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "kubagachi.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Should the ServiceAccount token be mounted?
Never in demo mode (so the entrypoint falls back to fake data); off in
kubeconfig mode (the token isn't used); otherwise honor serviceAccount.automount.
*/}}
{{- define "kubagachi.automountToken" -}}
{{- if eq .Values.mode "demo" -}}
false
{{- else if eq .Values.mode "kubeconfig" -}}
false
{{- else -}}
{{- .Values.serviceAccount.automount -}}
{{- end -}}
{{- end }}

{{/*
Name of the Secret holding kubeconfig data (kubeconfig mode).
*/}}
{{- define "kubagachi.kubeconfigSecretName" -}}
{{- if .Values.kubeconfig.existingSecret -}}
{{- .Values.kubeconfig.existingSecret -}}
{{- else -}}
{{- printf "%s-kubeconfig" (include "kubagachi.fullname" .) -}}
{{- end -}}
{{- end }}
