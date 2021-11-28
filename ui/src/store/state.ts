import { App } from '../client/models/App'

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
