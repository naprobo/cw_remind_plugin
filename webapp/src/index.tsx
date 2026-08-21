import React, {useEffect, useState} from 'react';
import {createPortal} from 'react-dom';
import {getMessages, localizeServerError, resolveLocale, SupportedLocale} from './i18n';
import './style.css';

const PLUGIN_ID = 'com.cw.remind';
type Rule = {days_before: number; time: string};
type StoredRule = Rule & {fire_at: number; fired_at?: number};
type ReminderUser = {id: string; username: string};
type Completion = {user_id: string; username: string; completed_at: number};
type Acknowledgement = {user_id: string; username: string; acknowledged_at: number};
type Reminder = {id: string; creator_id: string; creator_username: string; title: string; content: string; due_date: string; timezone: string; audience: string; usernames?: string[]; target_users?: ReminderUser[]; acknowledgements?: Acknowledgement[]; completions?: Completion[]; rules: StoredRule[]; created_at: number; updated_at?: number; deleted_at?: number};
type ChannelUser = {id: string; username: string; display_name: string};
type OpenData = {channelId: string} | null;
type MattermostState = {entities?: {general?: {config?: {DefaultClientLocale?: string}}; users?: {currentUserId?: string; profiles?: Record<string, {locale?: string}>}}};
type MattermostStore = {getState: () => MattermostState; subscribe: (listener: () => void) => () => void};
let openDialog: (channelId: string) => void = () => {};
let readLocale: () => SupportedLocale = () => 'en';
let readCurrentUserID: () => string = () => '';
let subscribeToLocale: (listener: () => void) => () => void = () => () => {};

function getCookie(name: string): string {
  const prefix = `${name}=`;
  const cookie = document.cookie.split(';').map((value) => value.trim()).find((value) => value.startsWith(prefix));
  return cookie ? cookie.slice(prefix.length) : '';
}

async function readResponse(response: Response): Promise<any> {
  const text = await response.text();
  if (!text) { return {}; }
  try {
    return JSON.parse(text);
  } catch {
    return {error: text.trim()};
  }
}

function useMattermostLocale(): SupportedLocale {
  const [locale, setLocale] = useState(readLocale);
  useEffect(() => subscribeToLocale(() => setLocale(readLocale())), []);
  return locale;
}

function Modal() {
  const locale = useMattermostLocale();
  const text = getMessages(locale);
  const [opened, setOpened] = useState<OpenData>(null);
  const [title, setTitle] = useState('');
  const [content, setContent] = useState('');
  const [dueDate, setDueDate] = useState('');
  const [audience, setAudience] = useState('all');
  const [selectedUsers, setSelectedUsers] = useState<ChannelUser[]>([]);
  const [userQuery, setUserQuery] = useState('');
  const [userOptions, setUserOptions] = useState<ChannelUser[]>([]);
  const [userPickerOpen, setUserPickerOpen] = useState(false);
  const [usersLoading, setUsersLoading] = useState(false);
  const [rules, setRules] = useState<Rule[]>([{days_before: 3, time: '09:00'}]);
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);
  const [view, setView] = useState<'create' | 'history'>('create');
  const [history, setHistory] = useState<Reminder[]>([]);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [historyRevision, setHistoryRevision] = useState(0);
  const [editingId, setEditingId] = useState<string | null>(null);
  useEffect(() => { openDialog = (channelId) => {setOpened({channelId}); setView('create'); setEditingId(null); setTitle(''); setContent(''); setDueDate(''); setAudience('all'); setSelectedUsers([]); setUserQuery(''); setRules([{days_before: 3, time: '09:00'}]); setError('');}; return () => {openDialog = () => {};}; }, []);
  useEffect(() => {
    if (!opened || audience !== 'users') { return; }
    const controller = new AbortController();
    const timer = window.setTimeout(async () => {
      setUsersLoading(true);
      try {
        const params = new URLSearchParams({channel_id: opened.channelId, q: userQuery});
        const response = await fetch(`/plugins/${PLUGIN_ID}/api/v1/channel-users?${params}`, {credentials: 'include', headers: {'X-Requested-With': 'XMLHttpRequest'}, signal: controller.signal});
        const body = await readResponse(response);
        if (!response.ok) { throw new Error(body.error || text.usersLoadFailed); }
        setUserOptions((body as ChannelUser[]).filter((candidate) => !selectedUsers.some((selected) => selected.id === candidate.id)));
      } catch (err) {
        if (!(err instanceof DOMException && err.name === 'AbortError')) { setError(err instanceof Error ? err.message : text.usersLoadFailed); }
      } finally {
        if (!controller.signal.aborted) { setUsersLoading(false); }
      }
    }, 200);
    return () => {window.clearTimeout(timer); controller.abort();};
  }, [opened?.channelId, audience, userQuery, selectedUsers.length, text.usersLoadFailed]);
  useEffect(() => {
    if (!opened || view !== 'history') { return; }
    const controller = new AbortController();
    setHistoryLoading(true); setError('');
    const params = new URLSearchParams({channel_id: opened.channelId});
    fetch(`/plugins/${PLUGIN_ID}/api/v1/reminders?${params}`, {credentials: 'include', headers: {'X-Requested-With': 'XMLHttpRequest'}, signal: controller.signal})
      .then(async (response) => ({response, body: await readResponse(response)}))
      .then(({response, body}) => {
        if (!response.ok) { throw new Error(body.error || text.historyLoadFailed); }
        setHistory(body as Reminder[]);
      })
      .catch((err) => {if (!(err instanceof DOMException && err.name === 'AbortError')) { setError(err instanceof Error ? err.message : text.historyLoadFailed); }})
      .finally(() => {if (!controller.signal.aborted) { setHistoryLoading(false); }});
    return () => controller.abort();
  }, [opened?.channelId, view, historyRevision, text.historyLoadFailed]);
  if (!opened) { return null; }
  const updateRule = (i: number, patch: Partial<Rule>) => setRules(rules.map((r, n) => n === i ? {...r, ...patch} : r));
  const submit = async (e: React.FormEvent) => {
    e.preventDefault(); setError(''); setSaving(true);
    if (audience === 'users' && selectedUsers.length === 0) { setError(text.userRequired); setSaving(false); return; }
    try {
      const csrfToken = getCookie('MMCSRF');
      const headers: Record<string, string> = {'Content-Type': 'application/json', 'X-Requested-With': 'XMLHttpRequest'};
      if (csrfToken) { headers['X-CSRF-Token'] = csrfToken; }
      const endpoint = editingId ? `/plugins/${PLUGIN_ID}/api/v1/reminders/${editingId}` : `/plugins/${PLUGIN_ID}/api/v1/reminders`;
      const response = await fetch(endpoint, {method: editingId ? 'PUT' : 'POST', credentials: 'include', headers, body: JSON.stringify({
        channel_id: opened.channelId, title, content, due_date: dueDate,
        timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC', audience,
        usernames: selectedUsers.map((user) => user.username), rules,
      })});
      const body = await readResponse(response);
      const responseError = body.error ? localizeServerError(body.error, locale) : response.statusText || text.saveFailed;
      if (!response.ok) {throw new Error(`HTTP ${response.status}: ${responseError}`);}
      setOpened(null); setTitle(''); setContent(''); setDueDate(''); setAudience('all'); setSelectedUsers([]); setUserQuery(''); setRules([{days_before: 3, time: '09:00'}]); setEditingId(null);
    } catch (err) { setError(err instanceof Error ? err.message : text.saveFailed); } finally { setSaving(false); }
  };
  const startEdit = (reminder: Reminder) => {
    setTitle(reminder.title || ''); setContent(reminder.content); setDueDate(reminder.due_date); setAudience(reminder.audience);
    setSelectedUsers(reminder.audience === 'users' ? (reminder.target_users || []).map((user) => ({...user, display_name: user.username})) : []);
    setRules(reminder.rules.map((rule) => ({days_before: rule.days_before, time: rule.time})));
    setEditingId(reminder.id); setView('create'); setError('');
  };
  const deleteReminder = async (reminder: Reminder) => {
    if (!window.confirm(text.deleteConfirm.replace('{title}', reminder.title || reminder.content))) { return; }
    setError('');
    const csrfToken = getCookie('MMCSRF');
    const headers: Record<string, string> = {'X-Requested-With': 'XMLHttpRequest'};
    if (csrfToken) { headers['X-CSRF-Token'] = csrfToken; }
    try {
      const response = await fetch(`/plugins/${PLUGIN_ID}/api/v1/reminders/${reminder.id}`, {method: 'DELETE', credentials: 'include', headers});
      const body = await readResponse(response);
      if (!response.ok) { throw new Error(body.error || text.deleteFailed); }
      setHistoryRevision((value) => value + 1);
    } catch (err) { setError(err instanceof Error ? err.message : text.deleteFailed); }
  };
  return createPortal(<div className='cw-remind-backdrop' onMouseDown={() => setOpened(null)}>
    <form className='cw-remind-modal' onSubmit={submit} onMouseDown={(e) => e.stopPropagation()}>
      <header><h2>{editingId ? text.editTitle : text.title}</h2><button type='button' className='cw-close' onClick={() => setOpened(null)} aria-label={text.close}>×</button></header>
      <nav className='cw-tabs'><button type='button' className={view === 'create' ? 'active' : ''} onClick={() => {setView('create'); setError('');}}>{text.createTab}</button><button type='button' className={view === 'history' ? 'active' : ''} onClick={() => {setView('history'); setError('');}}>{text.historyTab}</button></nav>
      {view === 'create' && <>
      <label>{text.reminderTitle}<input required maxLength={200} value={title} onChange={(e) => setTitle(e.target.value)} placeholder={text.titlePlaceholder}/></label>
      <label>{text.content}<textarea required maxLength={4000} value={content} onChange={(e) => setContent(e.target.value)} placeholder={text.contentPlaceholder}/></label>
      <label>{text.dueDate}<input required type='date' value={dueDate} onChange={(e) => setDueDate(e.target.value)}/></label>
      <fieldset><legend>{text.audience}</legend>
        {[['all', text.audienceAll], ['users', text.audienceUsers]].map(([v, label]) => <label className='cw-radio' key={v}><input type='radio' value={v} checked={audience === v} onChange={() => setAudience(v)}/>{label}</label>)}
      </fieldset>
      {audience === 'users' && <div className='cw-user-picker'><strong>{text.mattermostUsers}</strong>{selectedUsers.length > 0 && <div className='cw-selected-users' aria-label={text.selectedUsers}>{selectedUsers.map((user) => <span key={user.id}>@{user.username}<button type='button' aria-label={`${text.remove} @${user.username}`} onClick={() => setSelectedUsers(selectedUsers.filter((selected) => selected.id !== user.id))}>×</button></span>)}</div>}<input required={selectedUsers.length === 0} value={userQuery} onFocus={() => setUserPickerOpen(true)} onBlur={() => window.setTimeout(() => setUserPickerOpen(false), 150)} onChange={(e) => {setUserQuery(e.target.value); setUserPickerOpen(true);}} placeholder={text.searchUsers}/>{userPickerOpen && <div className='cw-user-options'>{usersLoading ? <div>{text.loading}</div> : userOptions.length === 0 ? <div>{text.noUsers}</div> : userOptions.map((user) => <button type='button' key={user.id} onMouseDown={(e) => e.preventDefault()} onClick={() => {setSelectedUsers([...selectedUsers, user]); setUserQuery('');}}><strong>@{user.username}</strong>{user.display_name !== user.username && <small>{user.display_name}</small>}</button>)}</div>}</div>}
      <section><div className='cw-rule-title'><strong>{text.reminderTimes}</strong><button type='button' onClick={() => setRules([...rules, {days_before: 0, time: '09:00'}])}>{text.addRule}</button></div>
        {rules.map((rule, i) => <div className='cw-rule' key={i}><span>{text.beforeDue}</span><input aria-label={text.daysBefore} type='number' min='0' max='3650' required value={rule.days_before} onChange={(e) => updateRule(i, {days_before: Number(e.target.value)})}/><span>{text.days}</span><input aria-label={text.time} type='time' required value={rule.time} onChange={(e) => updateRule(i, {time: e.target.value})}/><button type='button' disabled={rules.length === 1} onClick={() => setRules(rules.filter((_, n) => n !== i))}>{text.remove}</button></div>)}
      </section>
      </>}
      {view === 'history' && <section className='cw-history'><div className='cw-history-toolbar'><strong>{text.historyTab}</strong><button type='button' onClick={() => setHistoryRevision((value) => value + 1)}>{text.refresh}</button></div>{historyLoading ? <div className='cw-empty'>{text.loading}</div> : history.length === 0 ? <div className='cw-empty'>{text.noHistory}</div> : history.map((reminder) => {
        const fired = reminder.rules.filter((rule) => rule.fired_at).length;
        const status = reminder.deleted_at ? text.deleted : fired === reminder.rules.length ? text.sent : fired > 0 ? text.partiallySent : text.pending;
        const completed = reminder.completions || []; const acknowledged = reminder.acknowledgements || [];
        const completedIds = new Set(completed.map((item) => item.user_id)); const acknowledgedIds = new Set(acknowledged.map((item) => item.user_id));
        const unhandled = (reminder.target_users || []).filter((user) => !completedIds.has(user.id) && !acknowledgedIds.has(user.id));
        return <article key={reminder.id} className={reminder.deleted_at ? 'deleted' : ''}><div className='cw-history-heading'><strong>{reminder.title || text.legacyTitle}</strong><span className={`cw-status ${fired === reminder.rules.length ? 'sent' : ''}`}>{status}</span></div><p className='cw-history-content'>{reminder.content}</p><div className='cw-history-meta'><span>{text.createdAt}: {new Date(reminder.created_at).toLocaleString(locale)}</span><span>{text.creator}: @{reminder.creator_username}</span><span>{text.target}: {reminder.audience === 'users' ? reminder.usernames?.map((name) => `@${name}`).join(', ') : reminder.audience === 'channel' ? text.audienceChannel : text.audienceAll}</span>{reminder.target_users && <><span>{text.notHandled}: {unhandled.length ? unhandled.map((user) => `@${user.username}`).join(', ') : text.none}</span><span>{text.acknowledged}: {acknowledged.length ? acknowledged.map((item) => `@${item.username}`).join(', ') : text.none}</span><span>{text.handled}: {completed.length ? completed.map((item) => `@${item.username}`).join(', ') : text.none}</span></>}</div><div className='cw-history-rules'><strong>{text.schedule}</strong>{reminder.rules.map((rule, index) => <span key={`${rule.fire_at}-${index}`} className={rule.fired_at ? 'sent' : ''}>{new Date(rule.fire_at).toLocaleString(locale)} · {rule.fired_at ? text.sent : text.pending}</span>)}</div>{reminder.creator_id === readCurrentUserID() && !reminder.deleted_at && <div className='cw-history-actions'><button type='button' onClick={() => startEdit(reminder)}>{text.edit}</button><button type='button' className='danger' onClick={() => deleteReminder(reminder)}>{text.delete}</button></div>}</article>;
      })}</section>}
      {error && <div className='cw-error'>{error}</div>}
      <footer><button type='button' onClick={() => setOpened(null)}>{text.cancel}</button>{view === 'create' && <button className='cw-primary' disabled={saving} type='submit'>{saving ? text.saving : editingId ? text.saveChanges : text.create}</button>}</footer>
    </form>
  </div>, document.body);
}

class Plugin {
  initialize(registry: any, store: MattermostStore) {
    readLocale = () => {
      const state = store.getState();
      const users = state.entities?.users;
      const userLocale = users?.currentUserId ? users.profiles?.[users.currentUserId]?.locale : undefined;
      return resolveLocale(userLocale || state.entities?.general?.config?.DefaultClientLocale);
    };
    readCurrentUserID = () => store.getState().entities?.users?.currentUserId || '';
    subscribeToLocale = (listener) => store.subscribe(listener);
    registry.registerRootComponent(Modal);
    registry.registerWebSocketEventHandler(`custom_${PLUGIN_ID}_open`, (message: any) => openDialog(message.data.channel_id));
  }
}

declare global { interface Window {registerPlugin: (id: string, plugin: Plugin) => void;} }
window.registerPlugin(PLUGIN_ID, new Plugin());
