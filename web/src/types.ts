export type Phase =
  | "LOBBY"
  | "TEAM_PROPOSAL"
  | "PROPOSAL_SPEECH"
  | "TEAM_VOTE"
  | "QUEST_EXECUTION"
  | "ASSASSINATION"
  | "GAME_OVER";

export type Alignment = "GOOD" | "EVIL";

export type Role =
  | "MERLIN"
  | "PERCIVAL"
  | "SERVANT"
  | "ASSASSIN"
  | "MORGANA"
  | "MORDRED"
  | "OBERON"
  | "MINION";

export type WinReason =
  | "GOOD_WIN"
  | "EVIL_WIN_FIVE_REJECTS"
  | "EVIL_WIN_QUESTS"
  | "EVIL_WIN_ASSASSINATION";

export interface TeamieInitPayload {
  appId: string;
  slug: string;
  name: string;
  webUrl: string;
  mcpUrl?: string;
  token: string;
  expiresAt: string;
  workspaceId: string;
  conversationId: string;
  user: {
    id: string;
    displayName: string;
  };
}

export interface RoleOptions {
  merlin: boolean;
  percival: boolean;
  assassin: boolean;
  morgana: boolean;
  mordred: boolean;
  oberon: boolean;
}

export interface PublicChatPolicy {
  allowed: boolean;
  phaseMode: string;
  activeSpeakerSeat?: number;
  mustCallFinishSpeech?: boolean;
  guidance: string;
}

export interface PrivateVision {
  visibleEvilSeats?: number[];
  merlinCandidateSeats?: number[];
  evilTeammateSeats?: number[];
  visionSummary: string;
}

export interface SelfView {
  userId: string;
  displayName: string;
  seated: boolean;
  seat?: number;
  isHost?: boolean;
  isLeader?: boolean;
  isCurrentSpeaker?: boolean;
  onQuestTeam?: boolean;
  role?: Role;
  roleName?: string;
  alignment?: Alignment;
  privateVision?: PrivateVision;
  hasVotedTeam?: boolean;
  myTeamVote?: boolean;
  hasSubmittedQuest?: boolean;
  canActNow: boolean;
  actionRequired?: string;
  actionHint?: string;
}

export interface SeatView {
  seat: number;
  userId: string;
  displayName: string;
  isHost: boolean;
  isLeader: boolean;
  isCurrentSpeaker: boolean;
  onProposedTeam: boolean;
  hasSpokenThisTurn: boolean;
  hasVotedTeam: boolean;
  lastPublicVote?: boolean;
  revealedRole?: Role;
  revealedRoleName?: string;
  revealedAlignment?: Alignment;
}

export interface SeatVoteEntry {
  seat: number;
  displayName: string;
  approve: boolean;
}

export interface SpeechEntry {
  seat: number;
  displayName: string;
  summary?: string;
}

export interface ProposalRecord {
  questNumber: number;
  attemptNumber: number;
  leaderSeat: number;
  teamSeats: number[];
  votes: SeatVoteEntry[];
  approveCount: number;
  rejectCount: number;
  approved: boolean;
  speechNotes?: SpeechEntry[];
}

export interface QuestRecord {
  questNumber: number;
  teamSize: number;
  failsRequired: number;
  twoFailsRequired: boolean;
  leaderSeat: number;
  teamSeats: number[];
  successCount: number;
  failCount: number;
  succeeded: boolean;
}

export interface QuestProgressSlot {
  questNumber: number;
  teamSize: number;
  failsRequired: number;
  twoFailsRequired: boolean;
  status: "PENDING" | "ACTIVE" | "SUCCEEDED" | "FAILED";
  successCount?: number;
  failCount?: number;
}

export interface AssassinationRecord {
  assassinSeat: number;
  targetSeat: number;
  targetRole: Role;
  hitMerlin: boolean;
}

export interface PlayerView {
  workspaceId: string;
  conversationId: string;
  gameId?: string;
  eventSeq: number;
  phase: Phase;
  phaseLabel: string;
  preset: string;
  targetPlayerCount: number;
  goodCount: number;
  evilCount: number;
  roleOptions: RoleOptions;
  phaseTimeoutSec: number;
  phaseDeadlineUnixMs?: number;
  questNumber?: number;
  proposalAttempt?: number;
  rejectedCount: number;
  isHammerProposal?: boolean;
  leaderSeat?: number;
  currentTeamSize?: number;
  proposedTeamSeats?: number[];
  speechOrder?: number[];
  currentSpeakerSeat?: number;
  currentSpeechNotes?: SpeechEntry[];
  questSubmittedCount?: number;
  questRequiredCards?: number;
  questTrack: QuestProgressSlot[];
  completedQuests: QuestRecord[];
  voteHistory: ProposalRecord[];
  successQuestCount: number;
  failQuestCount: number;
  winner?: Alignment;
  winReason?: WinReason;
  winSummary?: string;
  assassination?: AssassinationRecord;
  seats: SeatView[];
  you: SelfView;
  publicChatPolicy: PublicChatPolicy;
  recentEvents: string[];
}

export interface ActionResponse {
  ok: boolean;
  eventSeq: number;
  message: string;
  phase: Phase;
  publicChatPolicy: PublicChatPolicy;
  view: PlayerView;
}
