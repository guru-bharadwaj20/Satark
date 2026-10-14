{{/*
Prometheus Operator ServiceMonitor scraping /metrics on the http port.
Off by default until the monitoring stack (Phase 6) provides the CRD.
Values: serviceMonitor.{enabled,interval,scrapeTimeout,labels}.
*/}}
{{- define "satark-common.servicemonitor" -}}
{{- if and .Values.serviceMonitor .Values.serviceMonitor.enabled }}
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: {{ include "satark-common.fullname" . }}
  labels:
    {{- include "satark-common.labels" . | nindent 4 }}
    {{- with .Values.serviceMonitor.labels }}
    {{- toYaml . | nindent 4 }}
    {{- end }}
spec:
  selector:
    matchLabels:
      {{- include "satark-common.selectorLabels" . | nindent 6 }}
  namespaceSelector:
    matchNames:
      - {{ .Release.Namespace }}
  endpoints:
    - port: http
      path: {{ .Values.serviceMonitor.path | default "/metrics" }}
      interval: {{ .Values.serviceMonitor.interval | default "30s" }}
      scrapeTimeout: {{ .Values.serviceMonitor.scrapeTimeout | default "10s" }}
{{- end }}
{{- end -}}
