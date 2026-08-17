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
  };
  tools: Tools;
  kernel_modules: string[];
  battery: BatteryIdentity;
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
