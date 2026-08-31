import { ApiError } from '@hyrox/api-client';
import { ArrowLeft } from 'lucide-react';
import { useState } from 'react';
import { useNavigate } from 'react-router';
import { api } from '../../lib/api';
import { useInvalidateAll, useMe } from '../../lib/queries';

export function EmergencyContactPage() {
  const navigate = useNavigate();
  const invalidate = useInvalidateAll();
  const { data: me } = useMe();
  const contact = me?.member.emergencyContact ?? null;
  const [name, setName] = useState(contact?.name ?? '');
  const [phone, setPhone] = useState(contact?.phone ?? '');
  const [relation, setRelation] = useState(contact?.relation ?? '');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  const save = async () => {
    setBusy(true);
    setError('');
    try {
      await api.me.update({
        emergencyContact: name && phone ? { name, phone, relation: relation || 'Contact' } : null,
      });
      invalidate();
      navigate(-1);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : 'Save failed.');
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="flex flex-col gap-5">
      <button onClick={() => navigate(-1)} className="flex items-center gap-1 text-sm font-bold text-muted">
        <ArrowLeft size={16} /> Back
      </button>
      <h1 className="display text-3xl">Emergency contact</h1>
      <p className="-mt-2 text-sm text-muted">
        Shown to studio staff if something happens during training.
      </p>
      <div>
        <label className="label">Contact name</label>
        <input className="input" value={name} onChange={(e) => setName(e.target.value)} />
      </div>
      <div>
        <label className="label">Contact phone</label>
        <input className="input" value={phone} onChange={(e) => setPhone(e.target.value)} placeholder="+62812…" />
      </div>
      <div>
        <label className="label">Relationship</label>
        <input className="input" value={relation} onChange={(e) => setRelation(e.target.value)} placeholder="Spouse, parent…" />
      </div>
      {error ? <p className="text-sm font-bold text-danger">{error}</p> : null}
      <button className="btn-brand" disabled={busy || !name || phone.length < 6} onClick={() => void save()}>
        Save contact
      </button>
    </div>
  );
}
