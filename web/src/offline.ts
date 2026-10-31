export interface PendingFile { blob: Blob; name: string; type: string }
export interface OutboxItem { id: string; text: string; files: PendingFile[]; idea_id?: string; created_at: string; error?: string }
const DB_NAME = 'com-itemgather-offline'
let dbPromise: Promise<IDBDatabase> | null = null

function openDB() {
  if (!dbPromise) dbPromise = new Promise((resolve, reject) => {
    const request = indexedDB.open(DB_NAME, 1)
    request.onupgradeneeded = () => { const db = request.result; if (!db.objectStoreNames.contains('drafts')) db.createObjectStore('drafts'); if (!db.objectStoreNames.contains('outbox')) db.createObjectStore('outbox', { keyPath: 'id' }) }
    request.onsuccess = () => resolve(request.result); request.onerror = () => reject(request.error)
  })
  return dbPromise
}
const plainFiles = (files: PendingFile[]) => files.map(file => ({ blob: file.blob, name: file.name, type: file.type }))
async function operate<T>(store: string, mode: IDBTransactionMode, action: (objectStore: IDBObjectStore) => IDBRequest<T>): Promise<T> {
  const db = await openDB()
  return new Promise((resolve, reject) => {
    const transaction = db.transaction(store, mode)
    const request = action(transaction.objectStore(store))
    transaction.oncomplete = () => resolve(request.result)
    request.onerror = () => reject(request.error)
    transaction.onabort = () => reject(transaction.error)
  })
}
export const saveDraft = (text: string, files: PendingFile[]) => operate('drafts', 'readwrite', store => store.put({ text, files: plainFiles(files) }, 'composer') as IDBRequest<any>)
export const getDraft = () => operate<{ text: string; files: PendingFile[] } | undefined>('drafts', 'readonly', store => store.get('composer') as IDBRequest<any>)
export const clearDraft = () => operate('drafts', 'readwrite', store => store.delete('composer') as IDBRequest<any>)
export const queueOutbox = (item: OutboxItem) => operate('outbox', 'readwrite', store => store.put({ ...item, files: plainFiles(item.files) }) as IDBRequest<any>)
export const listOutbox = () => operate<OutboxItem[]>('outbox', 'readonly', store => store.getAll() as IDBRequest<any>)
export const removeOutbox = (id: string) => operate('outbox', 'readwrite', store => store.delete(id) as IDBRequest<any>)
export const clearOffline = async () => {
  const db = await openDB()
  await Promise.all(['drafts', 'outbox'].map(name => new Promise<void>((resolve, reject) => {
    const transaction = db.transaction(name, 'readwrite')
    const request = transaction.objectStore(name).clear()
    transaction.oncomplete = () => resolve()
    request.onerror = () => reject(request.error)
    transaction.onabort = () => reject(transaction.error)
  })))
}
