import _ from 'lodash';
import { combineReducers } from 'redux';

import {
  CachedDataReducer, CachedDataReducerState, KeyedCachedDataReducer,
  KeyedCachedDataReducerState
} from './cachedDataReducers';
import * as api from '../utils/api';
import * as protos from '../proto/protos';


const networkListReducerObj = new CachedDataReducer(api.getNetworkList, 'networks');
export const refreshNetworks = networkListReducerObj.refresh;

const poolListReducerObj = new CachedDataReducer(api.getPoolList, 'pools');
export const refreshPools = poolListReducerObj.refresh;

const ipListReducerObj = new CachedDataReducer(api.getIPList, 'ips');
export const refreshIPs = ipListReducerObj.refresh;

const tempReservedIPListReducerObj = new CachedDataReducer(api.getTemporaryReservedIPList, 'temporaryReservedIPs');
export const refreshTempReservedIPs = tempReservedIPListReducerObj.refresh;

export const ipInPoolRequestToID = (req: protos.serverpb.GetIPInPoolRequest): string => `${req.rangeStart}/${req.rangeEnd}`

const ipsInPoolReducerObj = new KeyedCachedDataReducer(api.getIPInPool, 'ipsInPool', ipInPoolRequestToID);
export const refreshIPsInPool = ipsInPoolReducerObj.refresh;

const createIPReducerObj = new CachedDataReducer(api.createIP, 'createip');
export const createIP = createIPReducerObj.refresh;

const deactivateIPReducerbj = new CachedDataReducer(api.deactivateIP, 'deactivateip');
export const deactivateIP = deactivateIPReducerbj.refresh;

const updateIPReducerObj = new CachedDataReducer(api.updateIP, 'updateip');
export const updateIP = updateIPReducerObj.refresh;

const createPoolReducerObj = new CachedDataReducer(api.createPool, 'createPool');
export const createPool = createPoolReducerObj.refresh;

const updatePoolReducerObj = new CachedDataReducer(api.updatePool, 'updatepool');
export const updatePool = updatePoolReducerObj.refresh;

const deletePoolReducerObj = new CachedDataReducer(api.deletePool, `deletePool`);
export const deletePool = deletePoolReducerObj.refresh;

const createNetworkReducerObj = new CachedDataReducer(api.createNetwork, 'createNetwork');
export const createNetwork = createNetworkReducerObj.refresh;

const updateNetworkReducerObj = new CachedDataReducer(api.updateNetwork, 'updateNetwork');
export const updateNetwork = updateNetworkReducerObj.refresh;

const deleteNetworkReducerObj = new CachedDataReducer(api.deleteNetwork, 'deleteNetwork');
export const deleteNetwork = deleteNetworkReducerObj.refresh;

export const poolsInNetworkReqToID = (req: protos.serverpb.GetPoolsInNetworkRequest): string => `${req.ip}/${req.mask}`

const poolsInNetworkReducerObj = new KeyedCachedDataReducer(api.getPoolsInNetwork, 'poolsInNetwork', poolsInNetworkReqToID);
export const refreshPoolsInNetwork = poolsInNetworkReducerObj.refresh;

const drawIPReducerObj = new CachedDataReducer(api.drawIP, 'drawedIP');
export const drawIP = drawIPReducerObj.refresh;

export interface APIReducersState {
  networks: CachedDataReducerState<protos.serverpb.ListNetworkResponse>;
  pools: CachedDataReducerState<protos.serverpb.ListPoolResponse>;
  ips: CachedDataReducerState<protos.serverpb.ListIPResponse>;
  temporaryReservedIPs: CachedDataReducerState<protos.serverpb.ListTemporaryReservedIPResponse>;
  ipsInPool: KeyedCachedDataReducerState<protos.serverpb.GetIPInPoolResponse>;
  poolsInNetwork: KeyedCachedDataReducerState<protos.serverpb.GetPoolsInNetworkResponse>;
  drawedIP: CachedDataReducerState<protos.serverpb.DrawIPResponse>;
}

export default combineReducers<APIReducersState>({
  [networkListReducerObj.actionNamespace]: networkListReducerObj.reducer,
  [poolListReducerObj.actionNamespace]: poolListReducerObj.reducer,
  [ipListReducerObj.actionNamespace]: ipListReducerObj.reducer,
  [tempReservedIPListReducerObj.actionNamespace]: tempReservedIPListReducerObj.reducer,
  [ipsInPoolReducerObj.actionNamespace]: ipsInPoolReducerObj.reducer,
  [createIPReducerObj.actionNamespace]: createIPReducerObj.reducer,
  [deactivateIPReducerbj.actionNamespace]: deactivateIPReducerbj.reducer,
  [updateIPReducerObj.actionNamespace]: updateIPReducerObj.reducer,
  [createPoolReducerObj.actionNamespace]: createPoolReducerObj.reducer,
  [updatePoolReducerObj.actionNamespace]: updatePoolReducerObj.reducer,
  [deletePoolReducerObj.actionNamespace]: deletePoolReducerObj.reducer,
  [createNetworkReducerObj.actionNamespace]: createNetworkReducerObj.reducer,
  [updateNetworkReducerObj.actionNamespace]: updateNetworkReducerObj.reducer,
  [deleteNetworkReducerObj.actionNamespace]: deleteNetworkReducerObj.reducer,
  [poolsInNetworkReducerObj.actionNamespace]: poolsInNetworkReducerObj.reducer,
  [drawIPReducerObj.actionNamespace]: drawIPReducerObj.reducer,
});

export {
  CachedDataReducerState,
}
