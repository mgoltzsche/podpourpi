import {
  com_github_mgoltzsche_podpourpi_pkg_apis_app_v1alpha1_App as App,
} from '@/client'

export const state: State = {
  apps: [],
  getApp: function(name: string): App|null {
    return this.apps.find(a => a.metadata.name == name) || null
  }
}

export interface State {
  apps: App[]
  getApp(a: string): App|null
}
