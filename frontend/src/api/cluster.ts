import * as ClusterService from '../../bindings/rocket-leaf/internal/service/clusterservice.js'
import type { BrokerNode, ClusterInfo, ClusterSummary } from '../../bindings/rocket-leaf/internal/model/models.js'

export async function getBrokers(): Promise<(BrokerNode | null)[]> {
  try {
    return await ClusterService.GetBrokers()
  } catch (e) {
    console.error('GetBrokers', e)
    throw e
  }
}

export async function getClusterInfo(): Promise<ClusterInfo | null> {
  try {
    return await ClusterService.GetClusterInfo()
  } catch (e) {
    console.error('GetClusterInfo', e)
    throw e
  }
}

export async function getClusterSummary(): Promise<ClusterSummary | null> {
  try {
    return await ClusterService.GetClusterSummary()
  } catch (e) {
    console.error('GetClusterSummary', e)
    throw e
  }
}

export async function getBrokerDetail(brokerAddr: string): Promise<BrokerNode | null> {
  try {
    return await ClusterService.GetBrokerDetail(brokerAddr)
  } catch (e) {
    console.error('GetBrokerDetail', e)
    throw e
  }
}

export async function refreshBrokerStats(brokerAddr: string): Promise<BrokerNode | null> {
  try {
    return await ClusterService.RefreshBrokerStats(brokerAddr)
  } catch (e) {
    console.error('RefreshBrokerStats', e)
    throw e
  }
}
