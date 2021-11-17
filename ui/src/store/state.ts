import { App } from '../client/models/App'

export const state: State = {
    apps: [],
}

export interface State {
    apps: App[]
}
