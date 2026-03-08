import * as ConnectionService from '../../bindings/rocket-leaf/internal/service/connectionservice.js'
import type { Connection } from '../../bindings/rocket-leaf/internal/model/models.js'

export async function getConnections(): Promise<(Connection | null)[]> {
  try {
    return await ConnectionService.GetConnections()
  } catch (e) {
    console.error('GetConnections', e)
    throw e
  }
}

export async function addConnection(
  name: string,
  env: string,
  nameServer: string,
  timeoutSec: number,
  enableACL: boolean,
  accessKey: string,
  secretKey: string,
  remark: string
): Promise<Connection | null> {
  try {
    return await ConnectionService.AddConnection(name, env, nameServer, timeoutSec, enableACL, accessKey, secretKey, remark)
  } catch (e) {
    console.error('AddConnection', e)
    throw e
  }
}

export async function updateConnection(
  id: number,
  name: string,
  env: string,
  nameServer: string,
  timeoutSec: number,
  enableACL: boolean,
  accessKey: string,
  secretKey: string,
  remark: string
): Promise<Connection | null> {
  try {
    return await ConnectionService.UpdateConnection(id, name, env, nameServer, timeoutSec, enableACL, accessKey, secretKey, remark)
  } catch (e) {
    console.error('UpdateConnection', e)
    throw e
  }
}

export async function deleteConnection(id: number): Promise<void> {
  try {
    await ConnectionService.DeleteConnection(id)
  } catch (e) {
    console.error('DeleteConnection', e)
    throw e
  }
}

export async function connect(id: number): Promise<void> {
  try {
    await ConnectionService.Connect(id)
  } catch (e) {
    console.error('Connect', e)
    throw e
  }
}

export async function connectDefault(): Promise<void> {
  try {
    await ConnectionService.ConnectDefault()
  } catch (e) {
    console.error('ConnectDefault', e)
    throw e
  }
}

export async function setDefaultConnection(id: number): Promise<void> {
  try {
    await ConnectionService.SetDefaultConnection(id)
  } catch (e) {
    console.error('SetDefaultConnection', e)
    throw e
  }
}

export async function testConnection(id: number): Promise<string> {
  try {
    return await ConnectionService.TestConnection(id)
  } catch (e) {
    console.error('TestConnection', e)
    throw e
  }
}
