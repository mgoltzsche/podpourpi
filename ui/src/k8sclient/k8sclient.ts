/*import k8s from '@kubernetes/client-node';

const kc = new k8s.KubeConfig();

const cluster = {
    name: 'my-server',
    server: 'http://server.com',
};

const user = {
    name: 'my-user',
    password: 'some-password',
};

const context = {
    name: 'my-context',
    user: user.name,
    cluster: cluster.name,
};

export const client = kc.makeApiClient(k8s.CustomObjectsApi);
*/

import { request } from './request'
import { CancelablePromise } from './CancelablePromise'
import { WatchEvent, watch } from './watch'

export class KubeConfig {
  newClient<T extends Resource>(resourceUrl: string): ApiClient<T> {
    return new ApiClient<T>(resourceUrl)
  }
}

export class ApiClient<T extends Resource> {
  private resourceUrl: string
  constructor(resourceUrl: string) {
    this.resourceUrl = resourceUrl
  }
  public list(): CancelablePromise<Array<T>> {
    return request({
      method: 'GET',
      url: `${this.resourceUrl}`,
      query: {
        'allowWatchBookmarks': 1,
        'timeoutSeconds': 10,
      },
    })
  }
  public watch(handler: (evt:WatchEvent<T>)=>void): void {
    watch(`${this.resourceUrl}?watch=1&allowWatchBookmarks=1`, handler)
  }
  public update(obj: T): CancelablePromise<T> {
    return request({
      method: 'PUT',
      url: `${this.resourceUrl}/${obj.metadata.name||''}`,
      query: {
          'timeoutSeconds': 10,
      },
      body: obj
    })
  }
}

interface Resource {
  readonly metadata: ObjectMeta
}

interface ObjectMeta {
  readonly name?: string
}
