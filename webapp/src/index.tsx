import React, {useEffect, useState} from 'react';
import {createPortal} from 'react-dom';
import './style.css';

const PLUGIN_ID = 'com.cw.remind';
type Rule = {days_before: number; time: string};
type OpenData = {channelId: string} | null;
let openDialog: (channelId: string) => void = () => {};

function Modal() {
  const [opened, setOpened] = useState<OpenData>(null);
  const [content, setContent] = useState('');
  const [dueDate, setDueDate] = useState('');
  const [audience, setAudience] = useState('channel');
  const [users, setUsers] = useState('');
  const [rules, setRules] = useState<Rule[]>([{days_before: 3, time: '09:00'}]);
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);
  useEffect(() => { openDialog = (channelId) => {setOpened({channelId}); setError('');}; return () => {openDialog = () => {};}; }, []);
  if (!opened) { return null; }
  const updateRule = (i: number, patch: Partial<Rule>) => setRules(rules.map((r, n) => n === i ? {...r, ...patch} : r));
  const submit = async (e: React.FormEvent) => {
    e.preventDefault(); setError(''); setSaving(true);
    try {
      const response = await fetch(`/plugins/${PLUGIN_ID}/api/v1/reminders`, {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({
        channel_id: opened.channelId, content, due_date: dueDate,
        timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC', audience,
        usernames: users.split(',').map((v) => v.trim().replace(/^@/, '')).filter(Boolean), rules,
      })});
      const body = await response.json(); if (!response.ok) {throw new Error(body.error || '保存失败');}
      setOpened(null); setContent(''); setDueDate(''); setAudience('channel'); setUsers(''); setRules([{days_before: 3, time: '09:00'}]);
    } catch (err) { setError(err instanceof Error ? err.message : '保存失败'); } finally { setSaving(false); }
  };
  return createPortal(<div className='cw-remind-backdrop' onMouseDown={() => setOpened(null)}>
    <form className='cw-remind-modal' onSubmit={submit} onMouseDown={(e) => e.stopPropagation()}>
      <header><h2>设置期限提醒</h2><button type='button' className='cw-close' onClick={() => setOpened(null)} aria-label='关闭'>×</button></header>
      <label>提醒内容<textarea required maxLength={4000} value={content} onChange={(e) => setContent(e.target.value)} placeholder='例：提交月度报告'/></label>
      <label>执行期限<input required type='date' value={dueDate} onChange={(e) => setDueDate(e.target.value)}/></label>
      <fieldset><legend>提醒对象</legend>
        {[['all', 'All (@all)'], ['channel', 'Channel (@channel)'], ['users', '个别用户']].map(([v, label]) => <label className='cw-radio' key={v}><input type='radio' value={v} checked={audience === v} onChange={() => setAudience(v)}/>{label}</label>)}
      </fieldset>
      {audience === 'users' && <label>Mattermost 用户<input required value={users} onChange={(e) => setUsers(e.target.value)} placeholder='@alice, @bob'/><small>多个用户用逗号分隔</small></label>}
      <section><div className='cw-rule-title'><strong>提醒时间</strong><button type='button' onClick={() => setRules([...rules, {days_before: 0, time: '09:00'}])}>+追加一组</button></div>
        {rules.map((rule, i) => <div className='cw-rule' key={i}><span>期限前</span><input aria-label='提前天数' type='number' min='0' max='3650' required value={rule.days_before} onChange={(e) => updateRule(i, {days_before: Number(e.target.value)})}/><span>天</span><input aria-label='时间' type='time' required value={rule.time} onChange={(e) => updateRule(i, {time: e.target.value})}/><button type='button' disabled={rules.length === 1} onClick={() => setRules(rules.filter((_, n) => n !== i))}>删除</button></div>)}
      </section>
      {error && <div className='cw-error'>{error}</div>}
      <footer><button type='button' onClick={() => setOpened(null)}>取消</button><button className='cw-primary' disabled={saving} type='submit'>{saving ? '保存中…' : '建立提醒'}</button></footer>
    </form>
  </div>, document.body);
}

class Plugin {
  initialize(registry: any) {
    registry.registerRootComponent(Modal);
    registry.registerWebSocketEventHandler(`custom_${PLUGIN_ID}_open`, (message: any) => openDialog(message.data.channel_id));
  }
}

declare global { interface Window {registerPlugin: (id: string, plugin: Plugin) => void;} }
window.registerPlugin(PLUGIN_ID, new Plugin());
