{{/*
Deployment for a Satark service.
Values used: replicaCount, image.{repository,tag,digest,pullPolicy},
service.port, containerPort, env, extraEnv, envFrom, args, resources,
livenessProbe, readinessProbe, startupProbe, podAnnotations, nodeSelector,
tolerations, affinity, topologySpreadConstraints, volumes, volumeMounts,
serviceAccount, podSecurityContext, securityContext, autoscaling.enabled.
*/}}
{{- define "satark-common.deployment" -}}
{{- /* Every Satark service must declare probes and resources (task 3.17). */ -}}
{{- if not (and .Values.livenessProbe .Values.readinessProbe) -}}
{{- fail (printf "%s: livenessProbe and readinessProbe are required" .Chart.Name) -}}
{{- end -}}
{{- if not (and .Values.resources .Values.resources.requests .Values.resources.limits) -}}
{{- fail (printf "%s: resources.requests and resources.limits are required" .Chart.Name) -}}
{{- end -}}
{{- if not .Values.resources.limits.memory -}}
{{- fail (printf "%s: resources.limits.memory is required" .Chart.Name) -}}
{{- end -}}
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "satark-common.fullname" . }}
  labels:
    {{- include "satark-common.labels" . | nindent 4 }}
spec:
  {{- if not (and .Values.autoscaling .Values.autoscaling.enabled) }}
  replicas: {{ .Values.replicaCount | default 1 }}
  {{- end }}
  revisionHistoryLimit: 5
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 0
      maxSurge: 1
  selector:
    matchLabels:
      {{- include "satark-common.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      labels:
        {{- include "satark-common.selectorLabels" . | nindent 8 }}
        app.kubernetes.io/part-of: satark
      {{- with .Values.podAnnotations }}
      annotations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
    spec:
      serviceAccountName: {{ include "satark-common.serviceAccountName" . }}
      automountServiceAccountToken: {{ .Values.serviceAccount.automountToken | default false }}
      terminationGracePeriodSeconds: {{ .Values.terminationGracePeriodSeconds | default 30 }}
      securityContext:
        {{- include "satark-common.podSecurityContext" . | nindent 8 }}
      containers:
        - name: {{ include "satark-common.name" . }}
          image: {{ include "satark-common.image" . | quote }}
          imagePullPolicy: {{ .Values.image.pullPolicy | default "IfNotPresent" }}
          {{- with .Values.args }}
          args:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          ports:
            - name: http
              containerPort: {{ .Values.containerPort | default .Values.service.port }}
              protocol: TCP
          {{- $env := include "satark-common.env" . | trim }}
          {{- if $env }}
          env:
            {{- $env | nindent 12 }}
          {{- end }}
          {{- with .Values.envFrom }}
          envFrom:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          {{- with .Values.startupProbe }}
          startupProbe:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          {{- with .Values.livenessProbe }}
          livenessProbe:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          {{- with .Values.readinessProbe }}
          readinessProbe:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          {{- with .Values.resources }}
          resources:
            {{- toYaml . | nindent 12 }}
          {{- end }}
          securityContext:
            {{- include "satark-common.containerSecurityContext" . | nindent 12 }}
          {{- $tmp := include "satark-common.tmpVolumeEnabled" . }}
          {{- if or $tmp .Values.volumeMounts }}
          volumeMounts:
            {{- if $tmp }}
            - name: tmp
              mountPath: /tmp
            {{- end }}
            {{- with .Values.volumeMounts }}
            {{- toYaml . | nindent 12 }}
            {{- end }}
          {{- end }}
      {{- if or $tmp .Values.volumes }}
      volumes:
        {{- if $tmp }}
        - name: tmp
          emptyDir:
            sizeLimit: 64Mi
        {{- end }}
        {{- with .Values.volumes }}
        {{- toYaml . | nindent 8 }}
        {{- end }}
      {{- end }}
      {{- with .Values.nodeSelector }}
      nodeSelector:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.tolerations }}
      tolerations:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- with .Values.affinity }}
      affinity:
        {{- toYaml . | nindent 8 }}
      {{- end }}
      {{- if .Values.topologySpreadConstraints }}
      topologySpreadConstraints:
        {{- toYaml .Values.topologySpreadConstraints | nindent 8 }}
      {{- else }}
      {{- /* matchLabelKeys counts only pods from the same ReplicaSet. Without it
      the old pods count during a rollout and every new pod lands on the other
      node (seen in task 11.17). */}}
      topologySpreadConstraints:
        - maxSkew: 1
          topologyKey: kubernetes.io/hostname
          whenUnsatisfiable: ScheduleAnyway
          matchLabelKeys:
            - pod-template-hash
          labelSelector:
            matchLabels:
              {{- include "satark-common.selectorLabels" . | nindent 14 }}
      {{- end }}
{{- end -}}

{{- define "satark-common.serviceaccount" -}}
{{- if .Values.serviceAccount.create }}
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{ include "satark-common.serviceAccountName" . }}
  labels:
    {{- include "satark-common.labels" . | nindent 4 }}
automountServiceAccountToken: false
{{- end }}
{{- end -}}
