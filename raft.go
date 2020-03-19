package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import (
	"math/rand"
	"sync"
)
import "sync/atomic"
import "labrpc"

// import "bytes"
// import "labgob"

// Our own additional imports
import "time"

//
// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make(). set
// CommandValid to true to indicate that the ApplyMsg contains a newly
// committed log entry.
//
// in Lab 3 you'll want to send other kinds of messages (e.g.,
// snapshots) on the applyCh; at that point you can add fields to
// ApplyMsg, but set CommandValid to false for these other uses.
//
type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int
}

type StateType int

var CfgLogLevel LogLevel = 0

const (
	Follower  = iota
	Candidate = iota
	Leader    = iota
)

type LogEntry struct {
	Term    int
	Index   int
	Command interface{}
}

//
// A Go object implementing a single Raft peer.
//
type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]
	dead      int32               // set by Kill()

	// Your data here (2A, 2B, 2C).
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.
	State        StateType
	CurrentTerm  int
	VotedFor     int
	LastLogIndex int
	LastLogTerm  int
	LastTime     time.Time

	ApplyCh chan ApplyMsg

	Timer   *time.Timer
	Timeout time.Duration
	Ticker  *time.Ticker

	log         []LogEntry
	commitIndex int
	lastApplied int
	nextIndex   []int
	matchIndex  []int
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	var term int = rf.CurrentTerm
	var isleader bool = (rf.State == Leader)
	// Your code here (2A).
	return term, isleader
}

//
// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
//
func (rf *Raft) persist() {
	// Your code here (2C).
	// Example:
	// w := new(bytes.Buffer)
	// e := labgob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// data := w.Bytes()
	// rf.persister.SaveRaftState(data)
}

//
// restore previously persisted state.
//
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (2C).
	// Example:
	// r := bytes.NewBuffer(data)
	// d := labgob.NewDecoder(r)
	// var xxx
	// var yyy
	// if d.Decode(&xxx) != nil ||
	//    d.Decode(&yyy) != nil {
	//   error...
	// } else {
	//   rf.xxx = xxx
	//   rf.yyy = yyy
	// }
}

//
// example RequestVote RPC arguments structure.
// field names must start with capital letters!
//
type RequestVoteArgs struct {
	// Your data here (2A, 2B).
	Term         int
	CandidateId  int
	LastLogTerm  int
	LastLogIndex int
}

//
// example RequestVote RPC reply structure.
// field names must start with capital letters!
//
type RequestVoteReply struct {
	// Your data here (2A).
	Term        int
	VoteGranted bool
}

// 心跳请求
type AppendEntriesArgs struct {
	Term         int
	LeaderId     int
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []LogEntry
	LeaderCommit int
}

// 心跳响应
type AppendEntriesReply struct {
	Term    int
	Success bool
}

//
// example RequestVote RPC handler.
//
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (2A, 2B).
	// Receiver implemetation:
	// 1. Reply false if term < currentTerm
	// 2. If votedFor is null or candidateId and candidate's log is at
	//    least as up-to-date as receiver's log, grant vote
	rf.mu.Lock()
	defer rf.mu.Unlock()

	Logf(DEBUG, CfgLogLevel, rf.me, "receive RequestVote in term", rf.CurrentTerm)

	if args.Term < rf.CurrentTerm {
		reply.VoteGranted = false
		reply.Term = rf.CurrentTerm
		return
	}

	// In Rules for Servers
	// If RPC request or response contains term T > currentTerm:
	// set currentTerm = T, convert to follower

	if args.Term > rf.CurrentTerm {
		//Logf(DEBUG, CfgLogLevel, "RequestVote add Term",rf.me)
		//rf.Timer.Reset(rf.Timeout)
		rf.TurnToFollower(args.Term)
		// rf.LastTime = time.Now()
	}

	reply.Term = rf.CurrentTerm
	// If I am being asked to revote from the same candidate
	if ((rf.VotedFor == -1) || (rf.VotedFor == args.CandidateId)) &&
		((args.LastLogIndex >= rf.LastLogIndex) &&
			(args.LastLogTerm >= rf.LastLogTerm)) {
		Logf(DEBUG, CfgLogLevel, rf.me, "vote for", args.CandidateId, "in term", rf.CurrentTerm)
		rf.VotedFor = args.CandidateId
		reply.VoteGranted = true
	} else {
		reply.VoteGranted = false
	}
}

//
// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// The labrpc package simulates a lossy network, in which servers
// may be unreachable, and in which requests and replies may be lost.
// Call() sends a request and waits for a reply. If a reply arrives
// within a timeout interval, Call() returns true; otherwise
// Call() returns false. Thus Call() may not return for a while.
// A false return can be caused by a dead server, a live server that
// can't be reached, a lost request, or a lost reply.
//
// Call() is guaranteed to return (perhaps after a delay) *except* if the
// handler function on the server side does not return.  Thus there
// is no need to implement your own timeouts around Call().
//
// look at the comments in ../labrpc/labrpc.go for more details.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
//
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	Logf(DEBUG, CfgLogLevel, rf.me, " sendRequestVote ", "in term ", rf.CurrentTerm)
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}

// 心跳RPC开始

// 心跳请求
func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	//Logf(DEBUG, CfgLogLevel, rf.me, "sendAppendEntries in term", rf.CurrentTerm, "state is", rf.State)
	ok := rf.peers[server].Call("Raft.AppendEntriesHandler", args, reply)
	return ok
}

// 心跳响应
func (rf *Raft) AppendEntriesHandler(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	Logf(DEBUG, CfgLogLevel, rf.me, " receive AppendEntries from ", args.LeaderId, " in term", rf.CurrentTerm)

	//6.如果 leaderCommit > commitIndex，令 commitIndex 等于 leaderCommit 和 新日志条目索引值中较小的一个
	if args.LeaderCommit > rf.commitIndex {
		oldCommitIndex := rf.commitIndex
		if rf.LastLogIndex > args.LeaderCommit {
			rf.commitIndex = args.LeaderCommit
		} else {
			rf.commitIndex = rf.LastLogIndex
		}
		//如果commitIndex > lastApplied，那么就 lastApplied 加一，并把log[lastApplied]应用到状态机中（5.3 节）
		for i := oldCommitIndex+1; i <= rf.commitIndex; i++ {
			applyMsg := ApplyMsg{
				CommandValid: true,
				Command:      rf.log[i],
				CommandIndex: i,
			}
			rf.ApplyCh <- applyMsg
		}
	}

	//3.如果entries为空，响应心跳
	if len(args.Entries) == 0 {
		Logf(DEBUG, CfgLogLevel, rf.me, "args.Entries == 0 ")
		rf.TurnToFollower(args.Term)
		reply.Term = rf.CurrentTerm
		reply.Success = false
		return
	}
	// 1.处理follower log为空的情况
	if len(rf.log) == 0 {
		Logf(DEBUG, CfgLogLevel, rf.me, "rf.log == 0 ")
		rf.log = append(rf.log, args.Entries...)
		rf.LastLogIndex = len(rf.log) - 1
		Logf(DEBUG, CfgLogLevel, rf.me, " update log index to ", rf.LastLogIndex, " in term", rf.CurrentTerm, ". now log is ", rf.log)
		rf.TurnToFollower(args.Term)
		reply.Term = rf.CurrentTerm
		reply.Success = true
		return
	}

	//1.如果 term < currentTerm 就返回 false （5.1 节）
	//2.如果日志在 prevLogIndex 位置处的日志条目的任期号和 prevLogTerm 不匹配，则返回 false （5.3 节
	if args.Term < rf.CurrentTerm || rf.log[args.PrevLogIndex].Term != args.PrevLogTerm {
		reply.Term = rf.CurrentTerm
		reply.Success = false
		return
	}

	//4.如果已经存在的日志条目和新的产生冲突（索引值相同但是任期号不同），删除这一条和之后所有的 （5.3 节）
	if rf.LastLogIndex >= args.PrevLogIndex+1 {
		Logf(DEBUG, CfgLogLevel, rf.me, "冲突发生", "rf.LastLogIndex=", rf.LastLogIndex, "args.PrevLogIndex+1=", args.PrevLogIndex+1, ", logs", rf.log)
		if rf.log[args.PrevLogIndex+1].Term != args.Entries[0].Term {
			Logf(DEBUG, CfgLogLevel, rf.me, "发生截断")
			rf.log = rf.log[:args.PrevLogIndex]
		}
	}

	//5.附加日志中尚未存在的任何新条目
	rf.log = append(rf.log, args.Entries...)
	rf.LastLogIndex = len(rf.log) - 1
	Logf(DEBUG, CfgLogLevel, rf.me, " update log index to ", rf.LastLogIndex, " in term", rf.CurrentTerm, ". now log is ", rf.log)

	rf.TurnToFollower(args.Term)
	rf.CurrentTerm = args.Term
	reply.Term = rf.CurrentTerm
	reply.Success = true

	return
}

//
// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election. even if the Raft instance has been killed,
// this function should return gracefully.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
//
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	index := -1
	term := -1
	isLeader := true

	// Your code here (2B).
	if rf.State != Leader {
		return index, term, false
	}

	Logf(DEBUG, CfgLogLevel, rf.me, "receive a log ", command)
	entry := LogEntry{
		Term:    rf.CurrentTerm,
		Index:   len(rf.log),
		Command: command,
	}

	rf.log = append(rf.log, entry)
	rf.nextIndex[rf.me]++
	rf.matchIndex[rf.me]++
	Logf(DEBUG, CfgLogLevel, rf.me, "append log success, now log is", rf.log)

	return rf.commitIndex, term, isLeader
}

//
// the tester calls Kill() when a Raft instance won't
// be needed again. for your convenience, we supply
// code to set rf.dead (without needing a lock),
// and a killed() method to test rf.dead in
// long-running loops. you can also add your own
// code to Kill(). you're not required to do anything
// about this, but it may be convenient (for example)
// to suppress debug output from a Kill()ed instance.
//
func (rf *Raft) Kill() {
	atomic.StoreInt32(&rf.dead, 1)
	rf.mu.Lock()
	rf.Ticker = nil
	rf.mu.Unlock()
	// Your code here, if desired.
}

func (rf *Raft) killed() bool {
	z := atomic.LoadInt32(&rf.dead)
	return z == 1
}

func (rf *Raft) InitCandidate() {
	rf.mu.Lock()
	rf.State = Candidate
	rf.ResetTimer()
	rf.CurrentTerm++
	Logf(DEBUG, CfgLogLevel, rf.me, "start InitCandidate in term", rf.CurrentTerm)
	rf.VotedFor = rf.me
	voteCount := 1
	currentTerm := rf.CurrentTerm
	rf.mu.Unlock()

	req := &RequestVoteArgs{
		Term:         rf.CurrentTerm,
		CandidateId:  rf.me,
		LastLogTerm:  rf.LastLogTerm,
		LastLogIndex: rf.LastLogIndex,
	}

	replyHandler := func(reply *RequestVoteReply, peerID int, currentTerm int) {
		rf.mu.Lock()
		defer rf.mu.Unlock()

		if rf.CurrentTerm > currentTerm {
			return
		}
		// 对方term大于候选者，候选者直接变follower
		if reply.VoteGranted == false && reply.Term > rf.CurrentTerm {
			rf.TurnToFollower(reply.Term)
		}
		// 防止处理网络延迟的包
		if rf.State == Candidate { // else discard
			if reply.VoteGranted == true { // vote success
				voteCount++
				if voteCount*2 > len(rf.peers) {
					// no need to reset timer when become leader
					rf.Timer.Stop()
					go rf.InitLeader()
				}
			}
		}
	}

	for i := 0; i < len(rf.peers); i++ {
		if i == rf.me {
			continue
		}
		rsp := &RequestVoteReply{}

		go func(server int) {
			rf.sendRequestVote(server, req, rsp)
			replyHandler(rsp, server, currentTerm)
		}(i)

	}
}

func (rf *Raft) InitLeader() {
	Logf(DEBUG, CfgLogLevel, rf.me, " become leader in term", rf.CurrentTerm)
	rf.mu.Lock()
	rf.State = Leader
	rf.Ticker = time.NewTicker(time.Millisecond * 110)
	rf.mu.Unlock()

	// 处理网络延迟以及网络中断
	replyHandler := func(reply *AppendEntriesReply, peerID int, entriesLen int) {
		if rf.State != Leader {
			return
		}
		if reply.Term > rf.CurrentTerm {
			rf.TurnToFollower(reply.Term)
		}
		if reply.Success {
			rf.nextIndex[peerID] += entriesLen
			rf.matchIndex[peerID] += entriesLen
			//如果存在一个满足N > commitIndex的 N，并且大多数的matchIndex[i] ≥ N成立
			//并且log[N].term == currentTerm成立，那么令 commitIndex 等于这个 N （5.3 和 5.4 节）
			agreeCount := 1
			minMatchIndex := -1
			//初始化minMatchIndex
			//fmt.Println(rf.matchIndex)
			for _, v := range rf.matchIndex {
				if v >= rf.commitIndex {
					minMatchIndex = v
					break
				}
			}
			//fmt.Println("minMatchIndex",minMatchIndex)
			for _, v := range rf.matchIndex {
				if v >= rf.commitIndex {
					agreeCount++
					if v < minMatchIndex {
						minMatchIndex = v
					}
				}
			}

			if agreeCount > len(rf.peers)/2 && minMatchIndex >=0 && rf.log[minMatchIndex].Term == rf.CurrentTerm {
				//Logf(DEBUG, CfgLogLevel, rf.me, "start apply msg...")
				oldCommitIndex := rf.commitIndex
				rf.commitIndex = minMatchIndex
				//如果commitIndex > lastApplied，那么就 lastApplied 加一，并把log[lastApplied]应用到状态机中（5.3 节）
				for i := oldCommitIndex+1; i <= rf.commitIndex; i++ {
					//fmt.Println("start apply msg...")
					applyMsg := ApplyMsg{
						CommandValid: true,
						Command:      rf.log[i],
						CommandIndex: i,
					}
					rf.ApplyCh <- applyMsg
				}

			}
		}
	}

	// 周期性发送心跳
	for {
		if rf.State != Leader || rf.Ticker == nil {
			rf.ResetTimer()
			return
		}

		select {
		case <-rf.Ticker.C:
			for i := 0; i < len(rf.peers); i++ {
				if i == rf.me {
					continue
				}
				rf.mu.Lock()

				var req *AppendEntriesArgs

				req = &AppendEntriesArgs{}
				req.Term = rf.CurrentTerm
				req.LeaderId = rf.me
				req.Entries = rf.log[rf.nextIndex[i]:]

				if len(rf.log) == 0 || rf.nextIndex[i] == 0 {
					req.PrevLogIndex = 0
					req.PrevLogTerm = 0
					req.LeaderCommit = -1
				} else {
					req.PrevLogIndex = rf.nextIndex[i] - 1
					req.PrevLogTerm = rf.log[rf.nextIndex[i]-1].Term
					req.LeaderCommit = rf.commitIndex
				}

				rsp := &AppendEntriesReply{}
				entriesLen := len(req.Entries)
				rf.mu.Unlock()
				go func(p int) {
					Logf(DEBUG, CfgLogLevel, rf.me, "send AppendEntries to ", p, ", args:", req)
					rf.sendAppendEntries(p, req, rsp)
					replyHandler(rsp, p, entriesLen)
				}(i)
			}
		}
	}
}

func (rf *Raft) ResetTimer() {
	//rf.Timer.Stop()
	//rf.Timer = time.AfterFunc(rf.Timeout, func () {rf.InitCandidate()})
	rf.Timer.Reset(rf.Timeout)
}

func (rf *Raft) TurnToFollower(term int) {
	Logf(DEBUG, CfgLogLevel, rf.me, "turn to follower in term", term)
	if rf.State == Leader {
		rf.Ticker.Stop()
		rf.Ticker = nil
	}
	rf.State = Follower
	rf.CurrentTerm = term
	rf.VotedFor = -1
	rf.ResetTimer()
}

//
// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
//
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	Logf(DEBUG, CfgLogLevel, "Make invoke")
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me
	rf.CurrentTerm = 0
	rf.ApplyCh = applyCh

	rf.State = Follower
	rf.VotedFor = -1

	rf.LastLogIndex = 0
	rf.LastLogTerm = 1

	rf.lastApplied = -1
	rf.commitIndex = -1

	for i := 0; i < len(peers); i++ {
		rf.nextIndex = append(rf.nextIndex, len(rf.log))
		rf.matchIndex = append(rf.matchIndex, -1)
	}

	rf.log = []LogEntry{}

	rand.Seed(time.Now().UnixNano())
	rf.Timeout = time.Duration(int64(500)+rand.Int63n(500)) * time.Millisecond
	//go func(){
	//	rf.mu.Lock()
	rf.Timer = time.AfterFunc(rf.Timeout, func() { rf.InitCandidate() })
	//rf.mu.Unlock()
	//}()

	// Your initialization code here (2A, 2B, 2C).

	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	// go through peers
	// init each peer
	//
	return rf
}
