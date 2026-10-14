{{/*
PodDisruptionBudget, on by default (pdb.enabled). Defaults to
maxUnavailable: 1 so single-replica dev deployments can still drain; set
pdb.minAvailable to override.
*/}}
{{- define "satark-common.pdb" -}}
{{- if or (not (hasKey .Values "pdb")) .Values.pdb.enabled }}
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: {{ include "satark-common.fullname" . }}
  labels:
    {{- include "satark-common.labels" . | nindent 4 }}
spec:
  {{- if and .Values.pdb .Values.pdb.minAvailable }}
  minAvailable: {{ .Values.pdb.minAvailable }}
  {{- else }}
  maxUnavailable: {{ (and .Values.pdb .Values.pdb.maxUnavailable) | default 1 }}
  {{- end }}
  unhealthyPodEvictionPolicy: AlwaysAllow
  selector:
    matchLabels:
      {{- include "satark-common.selectorLabels" . | nindent 6 }}
{{- end }}
{{- end -}}
