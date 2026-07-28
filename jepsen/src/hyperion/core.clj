(ns hyperion.core
  (:gen-class)
  (:require [clojure.string :as str]
            [jepsen.checker :as checker]
            [jepsen.checker.timeline :as timeline]
            [jepsen.cli :as cli]
            [jepsen.client :as client]
            [jepsen.generator :as gen]
            [jepsen.tests :as tests]
            [knossos.model :as model])
  (:import (java.io DataInputStream DataOutputStream)
           (java.net InetSocketAddress Socket)
           (java.nio ByteBuffer ByteOrder)
           (java.util.zip CRC32C)))

(def addresses
  (delay (vec (str/split (or (System/getenv "HYPERION_CLIENTS")
                              "10.77.0.11:9200,10.77.0.12:9200,10.77.0.13:9200,10.77.0.14:9200,10.77.0.15:9200") #","))))

;; Node names and client addresses are positional: the Nth node name uses the
;; Nth entry of HYPERION_CLIENTS. Overridable so the same test can run against
;; the netns cluster on a Linux host and against loopback processes in CI.
(def nodes
  (delay (if-let [value (System/getenv "HYPERION_NODES")]
           (mapv keyword (str/split value #","))
           [:n1 :n2 :n3 :n4 :n5])))

;; Jepsen opens an SSH session per node unless told not to. The workload here
;; drives the cluster over TCP and never shells into a node, so a loopback run
;; sets HYPERION_JEPSEN_DUMMY_SSH=1 and skips it entirely.
(def dummy-ssh?
  (delay (= "1" (System/getenv "HYPERION_JEPSEN_DUMMY_SSH"))))

;; The register key this run operates on. Knossos checks a history against a
;; register that starts empty, so the key MUST NOT already hold a value when the
;; history begins. Reusing a fixed key against a cluster that survived an earlier
;; run makes the checker report a read of a value nothing in this history wrote,
;; which looks exactly like a linearizability violation and is not one. A key
;; unique per run removes the possibility entirely; set HYPERION_JEPSEN_KEY to
;; pin it when reproducing a recorded history.
(def register-key
  (delay (if-let [value (System/getenv "HYPERION_JEPSEN_KEY")]
           (Long/parseLong value)
           (inc (rem (System/nanoTime) 1000000000)))))

(defn le-buffer [size]
  (doto (ByteBuffer/allocate size) (.order ByteOrder/LITTLE_ENDIAN)))

(defn crc [^bytes payload]
  (let [value (CRC32C.)]
    (.update value payload 0 (alength payload))
    (.getValue value)))

(defn request-bytes [op client-id request-id key value]
  (let [buffer (le-buffer 40)]
    (.put buffer (byte op))
    (.position buffer 8)
    (.putLong buffer (long client-id))
    (.putLong buffer (long request-id))
    (.putLong buffer (long key))
    (.putLong buffer (long value))
    (.array buffer)))

(defn exchange [address payload]
  (let [[host port] (str/split address #":")
        socket (Socket.)]
    (.connect socket (InetSocketAddress. host (Integer/parseInt port)) 1000)
    (.setSoTimeout socket 3000)
    (with-open [socket socket
                out (DataOutputStream. (.getOutputStream socket))
                in (DataInputStream. (.getInputStream socket))]
      (let [header (doto (le-buffer 16)
                     (.put (.getBytes "HYPR" "UTF-8"))
                     (.putShort (short 1))
                     (.putShort (short 2))
                     (.putInt (alength payload))
                     (.putInt (unchecked-int (crc payload))))]
        (.write out (.array header))
        (.write out payload)
        (.flush out))
      (let [header (byte-array 16)]
        (.readFully in header)
        (let [h (doto (ByteBuffer/wrap header) (.order ByteOrder/LITTLE_ENDIAN))
              magic (byte-array 4)]
          (.get h magic)
          (when-not (= "HYPR" (String. magic "UTF-8"))
            (throw (ex-info "bad frame magic" {})))
          (when-not (= 1 (.getShort h))
            (throw (ex-info "bad protocol version" {})))
          (when-not (= 3 (.getShort h))
            (throw (ex-info "bad response kind" {})))
          (let [length (.getInt h)
                checksum (Integer/toUnsignedLong (.getInt h))
                response (byte-array length)]
            (.readFully in response)
            (when-not (= checksum (crc response))
              (throw (ex-info "bad frame checksum" {})))
            (doto (ByteBuffer/wrap response) (.order ByteOrder/LITTLE_ENDIAN))))))))

(defn invoke-node [node op client-id request-id key value]
  (let [response (exchange (@addresses node) (request-bytes op client-id request-id key value))
        status (Short/toUnsignedInt (.getShort response))
        _ (.position response 4)
        leader (.getInt response)
        _ (.getLong response)
        result (.getLong response)]
    {:status status :leader leader :value result}))

(defrecord HyperionClient [node client-id sequence]
  client/Client
  (open! [this _test node-name]
    (let [index (.indexOf ^java.util.List (:nodes _test) node-name)]
      (assoc this :node index :client-id (max 1 (bit-and Long/MAX_VALUE (System/nanoTime))) :sequence (atom 0))))
  (setup! [this _test] this)
  (invoke! [this _test op]
    (let [request-id (swap! sequence inc)
          wire-op (if (= :read (:f op)) 3 1)]
      (try
        (let [response (invoke-node node wire-op client-id request-id @register-key (or (:value op) 0))]
          (case (:status response)
            0 (assoc op :type :ok :value (if (= :read (:f op)) (:value response) (:value op)))
            1 (assoc op :type :fail :error :not-leader)
            2 (assoc op :type :fail :error :backpressure)
            4 (assoc op :type :ok :value nil)
            (assoc op :type :info :error [:server-status (:status response)])))
        (catch Exception exception
          (assoc op :type :info :error (.getMessage exception))))))
  (teardown! [this _test] this)
  (close! [this _test] this))

(defn read-op [_test _process] {:type :invoke :f :read :value nil})
(defn write-op [_test _process] {:type :invoke :f :write :value (rand-int 1000000)})

(defn hyperion-test [opts]
  (merge tests/noop-test
         opts
         {:name "hyperion-live-linearizability"
          :nodes @nodes
          :ssh {:dummy? @dummy-ssh?}
          :client (HyperionClient. nil nil nil)
          :model (model/register)
          :checker (checker/compose {:linearizable (checker/linearizable {:model (model/register)})
                                     :timeline (timeline/html)})
          :generator (gen/clients
                       (->> (gen/mix [read-op write-op])
                            (gen/stagger 0.1)
                            (gen/time-limit (:time-limit opts 60))))}))

(defn -main [& args]
  (cli/run! (merge (cli/single-test-cmd {:test-fn hyperion-test})
                   (cli/serve-cmd))
            args))
