export type Battery = {
  name: string;
  present: boolean;
  status?: string;
  capacity_percent?: number;
  voltage_now_v?: number;
  current_now_a?: number;
  power_now_w?: number;
  energy_full_wh?: number;
  energy_full_design_wh?: number;
  cycle_count?: number;
  power_w?: number;
  health_percent?: number;
};

export type Telemetry = {
  timestamp: string;
  provider: string;
  battery: Battery;
  missing_fields: string[];
  warnings?: string[];
};

export type Capabilities = {
  app: string;
  version: string;
  provider: string;
  listen: string;
  battery_present: boolean;
  battery_name?: string;
  sysfs_root?: string;
  available_fields: string[];
  features: {
    shutdown: boolean;
    docker: boolean;
    charge_thresholds: boolean;
    notifications: boolean;
    authentication: boolean;
  };
};
