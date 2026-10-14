{{/*
ClusterIP Service exposing the "http" container port.
Values: service.type (default ClusterIP), service.port, service.annotations.
*/}}
{{- define "satark-common.service" -}}
apiVersion: v1
kind: Service
metadata:
  name: {{ include "satark-common.fullname" . }}
  labels:
    {{- include "satark-common.labels" . | nindent 4 }}
  {{- with .Values.service.annotations }}
  annotations:
    {{- toYaml . | nindent 4 }}
  {{- end }}
spec:
  type: {{ .Values.service.type | default "ClusterIP" }}
  selector:
    {{- include "satark-common.selectorLabels" . | nindent 4 }}
  ports:
    - name: http
      port: {{ .Values.service.port }}
      targetPort: http
      protocol: TCP
{{- end -}}
