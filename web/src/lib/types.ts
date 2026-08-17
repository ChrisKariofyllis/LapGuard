export type Battery = {
  name: string;
  present: boolean;
  status?: string;
  capacity_percent?: number;
  capacity_level?: string;
  voltage_now_v?: number;
  current_now_a?: number;
  power_now_w?: number;
  energy_now_wh?: number;
  energy_full_wh?: number;
  energy_full_design_wh?: number;
  charge_now_ah?: number;
  charge_full_ah?: number;
  charge_full_design_ah?: number;
  cycle_count?: number;
  temperature_c?: number;
  manufacturer?: string;
  model_name?: string;
  serial_number?: string;
  technology?: string;
  charge_start_threshold?: number;
  charge_end_threshold?: number;
  power_w?: number;
  health_percent?: number;
  naming_convention?: string;
  power_calculation?: string;
  estimated_runtime_seconds?: number | null;
  estimated_runtime_hours?: number | null;
  estimated_runtime_available?: boolean;
  estimated_runtime_reason?: string | null;
};

export type Telemetry = {
  timestamp: string;
  provider: string;
  battery: Battery;
  available_fields?: string[];
  missing_fields: string[];
  warnings?: string[];
};

export type FeatureStatus = {
  key: string;
  label: string;
  enabled: boolean;
  detection_method: string;
  recommendation: string;
  why_not?: string;
  method?: string;
};

export type Tools = {
  tlp: boolean;
  tlp_version?: string;
  tlp_can_set_thresholds?: boolean;
  upower: boolean;
  acpi: boolean;
  tp_smapi?: boolean;
  i8kutils?: boolean;
};

export type BatteryIdentity = {
  path?: string;
  name?: string;
  present: boolean;
  manufacturer?: string;
  model?: string;
  serial?: string;
  technology?: string;
};

export type Capabilities = {
  app: string;
  version: string;
  provider: string;
  listen: string;
  battery_present: boolean;
  battery_name?: string;
  sysfs_root?: string;
  hostname?: string;
  os?: string;
  kernel?: string;
  available_fields: string[];
  naming_convention?: string;
  power_calculation?: string;
  threshold_method: string;
  features: FeatureStatus[];
  feature_flags: {
    charge_thresholds: string;
    cycle_count: boolean;
    power_now: boolean;
    raw_power_now_supported: boolean;
    derived_power_supported: boolean;
    current_voltage: boolean;
    temperature: boolean;
    alarm_control: boolean;
    docker_shutdown: boolean;
    power_loss_detection: boolean;
    outage_event_log: boolean;
    notifications: boolean;
    graceful_shutdown: boolean;
  };
  tools: Tools;
  kernel_modules: string[];
  battery: BatteryIdentity;
};

export type NotificationProvider = 'none' | 'ntfy' | 'telegram' | 'discord' | 'webhook';

export type NotificationsConfig = {
  provider: NotificationProvider;
  enabled: boolean;
  dry_run?: boolean;
  webhook_url: string;
  chat_id: string;
  webhook_configured?: boolean;
  chat_id_configured?: boolean;
};

export type ShutdownConfig = {
  enabled: boolean;
  warning_threshold: number;
  critical_threshold: number;
};

export type DockerConfig = {
  stop_enabled: boolean;
  timeout_seconds: number;
};

export type AppConfig = {
  notifications: NotificationsConfig;
  shutdown: ShutdownConfig;
  docker: DockerConfig;
  execution: {
    notifications: string;
    shutdown: string;
    docker: string;
  };
  notes?: string[];
};

export type PowerAdapter = {
  name: string;
  type: string;
  online: boolean | null;
  readable: boolean;
};

export type PowerStatus = {
  timestamp: string;
  source: 'AC' | 'BATTERY' | 'UNKNOWN';
  adapters: PowerAdapter[];
  reason?: string;
  watcher: {
    running: boolean;
    interval_seconds: number;
    debounce_seconds: number;
    last_poll?: string;
    baseline_recorded: boolean;
    pending_source?: string;
  };
};

export type PowerEvent = {
  id: number;
  type: 'AC_CONNECTED' | 'AC_DISCONNECTED' | 'AC_UNKNOWN' | string;
  timestamp: string;
  source: string;
  duration_ms?: number;
};

export type EventsResponse = {
  events: PowerEvent[];
  available: boolean;
};

export type DiscoverReport = {
  timestamp: string;
  hostname: string;
  os: string;
  kernel: string;
  battery: BatteryIdentity;
  available_fields: string[];
  naming_convention?: string;
  power_calculation?: string;
  features: Capabilities['feature_flags'];
  available_tools: Tools;
  kernel_modules: string[];
  thresholds: {
    method: string;
    detection_method: string;
    recommendation?: string;
    why_not?: string;
    start?: number;
    end?: number;
  };
  notes?: string[];
};
