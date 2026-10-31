export interface PendingFile { id: string; blob: Blob; name: string; type: string }
export interface OutboxItem { id: string; user_id: string; text: string; files: PendingFile[]; idea_id?: string; created_at: string; error?: string }
export interface OfflineScope { readonly userID: string }
const DB_NAME = 'com-itemgather-offline'
let dbPromise: Promise<IDBDatabase> | null = null
let activeScope: OfflineScope | null = null

export function bindOfflineUser(userID: string) { activeScope = { userID } }
export function unbindOfflineUser() { activeScope = null }
export function currentOfflineScope(): OfflineScope {
  if (!activeScope) throw new Error('请先登录再使用本机草稿')
  return activeScope
}
export const isOfflineScopeActive = (scope: OfflineScope) => activeScope === scope
function requireScope(scope: OfflineScope) {
  if (!isOfflineScopeActive(scope)) throw new Error('账号已变化，已停止上一账号的同步')
}

function openDB(): Promise<IDBDatabase> {
  if (!dbPromise) dbPromise = new Promise((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, 2)
    request.onupgradeneeded = () => {
      const db = request.result
      if (!db.objectStoreNames.contains('drafts')) db.createObjectStore('drafts')
      const outbox = db.objectStoreNames.contains('outbox') ? request.transaction!.objectStore('outbox') : db.createObjectStore('outbox', { keyPath: 'id' })
      if (!outbox.indexNames.contains('user_id')) outbox.createIndex('user_id', 'user_id')
      // Retain legacy records for recovery, without adopting their unknown owner.
    }
    request.onsuccess = () => { const db = request.result; db.onversionchange = () => { db.close(); dbPromise = null }; resolve(db) }
    request.onerror = () => { dbPromise = null; reject(request.error) }
    request.onblocked = () => { dbPromise = null; reject(new Error('请关闭此站点的旧标签页后重试本机草稿')) }
  })
  return dbPromise
}
const plainFiles = (files: PendingFile[]) => files.map(({ id, blob, name, type }) => ({ id, blob, name, type }))
async function operate<T>(scope: OfflineScope, store: string, mode: IDBTransactionMode, action: (objectStore: IDBObjectStore) => IDBRequest<T>): Promise<T> {
  requireScope(scope)
  const db = await openDB()
  requireScope(scope)
  return new Promise((resolve, reject) => {
    const transaction = db.transaction(store, mode)
    const request = action(transaction.objectStore(store))
    transaction.oncomplete = () => resolve(request.result)
    request.onerror = () => reject(request.error)
    transaction.onabort = () => reject(transaction.error)
  })
}
const draftKey = (userID: string) => `composer:${userID}`
export const saveDraft = (scope: OfflineScope, text: string, files: PendingFile[]) => operate(scope, 'drafts', 'readwrite', store => store.put({ user_id: scope.userID, text, files: plainFiles(files) }, draftKey(scope.userID)))
export const getDraft = (scope: OfflineScope) => operate<{ text: string; files: PendingFile[] } | undefined>(scope, 'drafts', 'readonly', store => store.get(draftKey(scope.userID)))
export const clearDraft = (scope: OfflineScope) => operate(scope, 'drafts', 'readwrite', store => store.delete(draftKey(scope.userID)))
export function queueOutbox(scope: OfflineScope, item: OutboxItem) {
  if (item.user_id !== scope.userID) throw new Error('待同步内容属于另一账号')
  return operate(scope, 'outbox', 'readwrite', store => store.put({ ...item, files: plainFiles(item.files) }))
}
export const listOutbox = (scope: OfflineScope) => operate<OutboxItem[]>(scope, 'outbox', 'readonly', store => store.index('user_id').getAll(scope.userID))
export const removeOutbox = (scope: OfflineScope, id: string) => operate(scope, 'outbox', 'readwrite', store => store.delete(id))
export const clearOffline = async (userID: string) => {
  const db = await openDB()
  return new Promise<void>((resolve, reject) => {
    const transaction = db.transaction(['drafts', 'outbox'], 'readwrite')
    transaction.objectStore('drafts').delete(draftKey(userID))
    transaction.objectStore('drafts').delete('composer')
    const request = transaction.objectStore('outbox').openCursor()
    request.onsuccess = () => {
      const cursor = request.result
      if (!cursor) return
      if (!cursor.value.user_id || cursor.value.user_id === userID) cursor.delete()
      cursor.continue()
    }
    transaction.oncomplete = () => resolve()
    transaction.onabort = () => reject(transaction.error)
    transaction.onerror = () => reject(transaction.error)
  })
}
