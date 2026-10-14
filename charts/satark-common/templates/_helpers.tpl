{{/*
Common helpers. A service chart depends on satark-common and renders
resources with e.g. {{ include "satark-common.deployment" . }}.
*/}}

{{- define "satark-common.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "satark-common.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- include "satark-common.name" . -}}
{{- end -}}
{{- end -}}

{{- define "satark-common.selectorLabels" -}}
app.kubernetes.io/name: {{ include "satark-common.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "satark-common.labels" -}}
{{ include "satark-common.selectorLabels" . }}
app.kubernetes.io/version: {{ .Values.image.tag | default .Chart.AppVersion | quote }}
app.kubernetes.io/part-of: satark
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end -}}

{{- define "satark-common.image" -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion -}}
{{- if .Values.image.digest -}}
{{- printf "%s@%s" .Values.image.repository .Values.image.digest -}}
{{- else -}}
{{- printf "%s:%s" .Values.image.repository $tag -}}
{{- end -}}
{{- end -}}

{{- define "satark-common.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "satark-common.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{/*
env accepts either a map (NAME: value) or a list of EnvVar objects.
*/}}
{{- define "satark-common.env" -}}
{{- if kindIs "map" .Values.env -}}
{{- range $k, $v := .Values.env }}
- name: {{ $k }}
  value: {{ $v | toString | quote }}
{{- end }}
{{- else if .Values.env -}}
{{ toYaml .Values.env }}
{{- end -}}
{{- with .Values.extraEnv }}
{{ toYaml . }}
{{- end -}}
{{- end -}}
