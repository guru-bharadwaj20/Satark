{{/*
HorizontalPodAutoscaler (autoscaling/v2), rendered when autoscaling.enabled.
Values: autoscaling.{minReplicas,maxReplicas,targetCPUUtilizationPercentage,
targetMemoryUtilizationPercentage}. Scale-down is damped to avoid flapping.
*/}}
{{- define "satark-common.hpa" -}}
{{- if and .Values.autoscaling .Values.autoscaling.enabled }}
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: {{ include "satark-common.fullname" . }}
  labels:
    {{- include "satark-common.labels" . | nindent 4 }}
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: {{ include "satark-common.fullname" . }}
  minReplicas: {{ .Values.autoscaling.minReplicas | default 2 }}
  maxReplicas: {{ .Values.autoscaling.maxReplicas | default 5 }}
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: {{ .Values.autoscaling.targetCPUUtilizationPercentage | default 70 }}
    {{- with .Values.autoscaling.targetMemoryUtilizationPercentage }}
    - type: Resource
      resource:
        name: memory
        target:
          type: Utilization
          averageUtilization: {{ . }}
    {{- end }}
  behavior:
    scaleDown:
      stabilizationWindowSeconds: 300
      policies:
        - type: Pods
          value: 1
          periodSeconds: 60
{{- end }}
{{- end -}}
