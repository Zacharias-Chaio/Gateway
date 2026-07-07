/* ══════════════ Constants ══════════════ */
const TOTAL_STEPS = 3;
const CH_TOTAL_STEPS = 3;
const PROTOCOLS_BY_INTERFACE = {
  Serial: ['Modbus RTU', 'IEC103', 'DL/T 645-1997', 'DL/T 645-2007', '自定义协议'],
  Network: ['Modbus TCP', 'Modbus RTU', 'IEC103', 'IEC104', 'IEC61850', 'OPC UA', 'MQTT', 'HTTP/HTTPS', '自定义协议'],
  CAN: ['CAN协议', '自定义协议']
};
const DT_LABEL = { bool:'布尔', bitmap:'位图', enum:'枚举', int:'整数', float:'浮点数', string:'字符串' };
const DT_FROM_LABEL = Object.fromEntries(Object.entries(DT_LABEL).map(([k, v]) => [v, k]));
const ACCESS_LABEL = { r:'只读', w:'只写', rw:'读写' };
const ACCESS_FROM_LABEL = { '只读':'r', '只写':'w', '读写':'rw', 'R':'r', 'W':'w', 'RW':'rw', 'r':'r', 'w':'w', 'rw':'rw' };
const CHANNEL_TYPE_LABEL = { Serial:'串口通道', Network:'网络通道', CAN:'CAN通道' };
const CHANNEL_TYPE_ICON = { Serial:'usb-symbol', Network:'ethernet', CAN:'hdd-network' };
const PARITY_LABEL = { None:'无', Even:'偶校验', Odd:'奇校验' };
const DEFAULT_HARDWARE = {
  Serial: { COM1: '/dev/ttyS1', COM2: '/dev/ttyS2' },
  Ethernet: { ETH1: 'eth0', ETH2: 'eth2' },
  CAN: { CAN1: 'can0', CAN2: 'can1' }
};

const CSV_HEADERS = ['属性ID','属性名称','属性描述','数据类型','数据长度','读写属性','数据基数','数据系数','数据单位','读功能码','写功能码','寄存器基址','寄存器偏移','字节顺序'];
const CSV_FIELD_MAP = {
  '属性ID':'id','id':'id',
  '属性名称':'name','名称':'name','name':'name',
  '属性描述':'description','描述':'description','description':'description',
  '数据类型':'dataType','datatype':'dataType',
  '数据长度':'dataLength','datalength':'dataLength',
  '数据单位':'unit','单位':'unit','unit':'unit',
  '读写属性':'accessMode','读写':'accessMode','accessmode':'accessMode',
  '数据基数':'base','基数':'base','base':'base',
  '数据系数':'coefficient','系数':'coefficient','coefficient':'coefficient',
  '读功能码':'readFunctionCode','readfunctioncode':'readFunctionCode',
  '写功能码':'writeFunctionCode','writefunctioncode':'writeFunctionCode',
  '寄存器基址':'registerBase','寄存器地址':'registerBase','registerbase':'registerBase','registeraddress':'registerBase',
  '寄存器偏移':'registerOffset','位偏移':'registerOffset','registeroffset':'registerOffset','bitoffset':'registerOffset',
  '字节顺序':'byteOrder','byteorder':'byteOrder'
};

/* ══════════════ State ══════════════ */
const state = {
  models: [],
  hardware: {},
  channels: [],
  editingId: null,
  channel: null,
  profile: { profileIndex:'', profileId:'', name:'', manufacturer:'', description:'', deviceType:'', deviceModel:'', ratedPower:'', interfaceType:'', protocolType:'', protocolVersion:'', maxRegisterCount: 125 },
  properties: []
};
let currentStep = 1;
let chCurrentStep = 1;
let propEditIndex = -1;
let channelEditIndex = -1;
let propModal;

